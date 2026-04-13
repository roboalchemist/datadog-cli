package cmd

// apm endpoints — lists per-resource (endpoint) stats for an APM service.
//
// There is no single Datadog REST endpoint that returns per-resource APM stats
// as a flat list (as of v1/v2 API specs).  The closest primitives are:
//
//   • GET /api/v1/query (timeseries) — aggregates trace.* metrics by resource_name
//   • GET /api/v2/apm/services         — lists services + metadata
//   • Dashboard "apm_resource_stats"   — widget query definition only, not a REST call
//
// This command queries the timeseries API three times (hits, errors, latency)
// and joins the results by resource_name scope tag.  The approach mirrors what
// Datadog's built-in APM service page does under the hood.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- flags ----

var (
	apmEndpointsService  string
	apmEndpointsEnv      string
	apmEndpointsFrom     string
	apmEndpointsTo       string
	apmEndpointsResource string
	apmEndpointsSort     string
)

// ---- command definition ----

var apmEndpointsCmd = &cobra.Command{
	Use:   "endpoints",
	Short: "List APM endpoint (resource) stats for a service",
	Long: `List per-endpoint (resource) stats for an APM service.

Queries the Datadog timeseries API (GET /api/v1/query) for the three core
trace metrics — hits/s, errors/s, and avg latency — then joins them by
resource_name tag.

Note: Datadog does not expose a dedicated REST endpoint for per-resource APM
stats.  This command approximates the APM service page by querying:
  trace.web.request.hits{service:<SVC>,env:<ENV>} by {resource_name}
  trace.web.request.errors{service:<SVC>,env:<ENV>} by {resource_name}
  trace.web.request.duration{service:<SVC>,env:<ENV>} by {resource_name}

If these metrics are empty try using --span-kind=db (trace.db.query.*) or
any custom span name your service uses.`,
	Example: `  # List endpoints for the api-gateway service in production (last hour)
  datadog-cli apm endpoints --service api-gateway --env production

  # Filter to resources matching a pattern
  datadog-cli apm endpoints --service api-gateway --env production --resource "/api/v1"

  # Sort by error rate
  datadog-cli apm endpoints --service api-gateway --env production --sort errors

  # Custom time window
  datadog-cli apm endpoints --service api-gateway --env production --from 3h --to now

  # JSON output
  datadog-cli apm endpoints --service api-gateway --env production --json`,
	RunE: runAPMEndpoints,
}

// endpointRow holds the joined stats for a single resource_name.
type endpointRow struct {
	Resource   string
	HitsPerSec string
	ErrorRate  string
	AvgLatency string
}

func runAPMEndpoints(cmd *cobra.Command, args []string) error {
	if apmEndpointsService == "" {
		return fmt.Errorf("--service is required")
	}

	fromSecs, err := parseTimeSeconds(apmEndpointsFrom)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}
	toSecs, err := parseTimeSeconds(apmEndpointsTo)
	if err != nil {
		return fmt.Errorf("--to: %w", err)
	}
	if fromSecs >= toSecs {
		return fmt.Errorf("--from time must be before --to time")
	}

	client := newClient()
	opts := GetOutputOptions()

	scope := fmt.Sprintf("service:%s", apmEndpointsService)
	if apmEndpointsEnv != "" {
		scope += fmt.Sprintf(",env:%s", apmEndpointsEnv)
	}

	// Query helper: runs GET /api/v1/query and returns parsed series list.
	queryMetric := func(q string) ([]interface{}, error) {
		params := url.Values{}
		params.Set("query", q)
		params.Set("from", strconv.FormatInt(fromSecs, 10))
		params.Set("to", strconv.FormatInt(toSecs, 10))

		respBytes, err := client.Get("/api/v1/query", params)
		if err != nil {
			return nil, err
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(respBytes, &raw); err != nil {
			return nil, fmt.Errorf("parsing response: %w", err)
		}

		series, _ := raw["series"].([]interface{})
		return series, nil
	}

	// avgSeriesValue returns the time-averaged value of a series pointlist.
	avgSeriesValue := func(series map[string]interface{}) float64 {
		pl, _ := series["pointlist"].([]interface{})
		if len(pl) == 0 {
			return 0
		}
		sum := 0.0
		count := 0
		for _, pt := range pl {
			pair, _ := pt.([]interface{})
			if len(pair) >= 2 {
				if v, ok := pair[1].(float64); ok {
					sum += v
					count++
				}
			}
		}
		if count == 0 {
			return 0
		}
		return sum / float64(count)
	}

	// resourceName extracts the resource_name tag from a series scope string.
	// Scope looks like "env:production,resource_name:GET /api/v1/users,service:api-gateway"
	resourceName := func(series map[string]interface{}) string {
		scopeStr, _ := series["scope"].(string)
		for _, part := range strings.Split(scopeStr, ",") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "resource_name:") {
				return strings.TrimPrefix(part, "resource_name:")
			}
		}
		// Fall back to the full expression as display name
		expr, _ := series["expression"].(string)
		if expr != "" {
			return expr
		}
		return scopeStr
	}

	// ---- fetch hits ----
	hitsQuery := fmt.Sprintf("sum:trace.web.request.hits{%s} by {resource_name}", scope)
	hitsSeries, err := queryMetric(hitsQuery)
	if err != nil {
		return fmt.Errorf("querying hits: %w", err)
	}

	// ---- fetch errors ----
	errorsQuery := fmt.Sprintf("sum:trace.web.request.errors{%s} by {resource_name}", scope)
	errorsSeries, err := queryMetric(errorsQuery)
	if err != nil {
		return fmt.Errorf("querying errors: %w", err)
	}

	// ---- fetch latency ----
	latencyQuery := fmt.Sprintf("avg:trace.web.request.duration{%s} by {resource_name}", scope)
	latencySeries, err := queryMetric(latencyQuery)
	if err != nil {
		return fmt.Errorf("querying latency: %w", err)
	}

	// ---- join series by resource_name ----
	type stats struct {
		hits    float64
		errors  float64
		latency float64 // nanoseconds
	}
	data := map[string]*stats{}

	for _, s := range hitsSeries {
		m, _ := s.(map[string]interface{})
		r := resourceName(m)
		if _, ok := data[r]; !ok {
			data[r] = &stats{}
		}
		data[r].hits = avgSeriesValue(m)
	}
	for _, s := range errorsSeries {
		m, _ := s.(map[string]interface{})
		r := resourceName(m)
		if _, ok := data[r]; !ok {
			data[r] = &stats{}
		}
		data[r].errors = avgSeriesValue(m)
	}
	for _, s := range latencySeries {
		m, _ := s.(map[string]interface{})
		r := resourceName(m)
		if _, ok := data[r]; !ok {
			data[r] = &stats{}
		}
		data[r].latency = avgSeriesValue(m)
	}

	// ---- apply --resource filter ----
	if apmEndpointsResource != "" {
		filter := strings.ToLower(apmEndpointsResource)
		for k := range data {
			if !strings.Contains(strings.ToLower(k), filter) {
				delete(data, k)
			}
		}
	}

	// ---- sort ----
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	switch strings.ToLower(apmEndpointsSort) {
	case "hits", "throughput":
		sort.Slice(keys, func(i, j int) bool {
			return data[keys[i]].hits > data[keys[j]].hits
		})
	case "errors", "error_rate", "error-rate":
		sort.Slice(keys, func(i, j int) bool {
			di, dj := data[keys[i]], data[keys[j]]
			ri, rj := 0.0, 0.0
			if di.hits > 0 {
				ri = di.errors / di.hits
			}
			if dj.hits > 0 {
				rj = dj.errors / dj.hits
			}
			return ri > rj
		})
	case "latency", "p50", "avg":
		sort.Slice(keys, func(i, j int) bool {
			return data[keys[i]].latency > data[keys[j]].latency
		})
	default:
		// alphabetical by resource name
		sort.Strings(keys)
	}

	// Apply limit
	if flagLimit > 0 && len(keys) > flagLimit {
		keys = keys[:flagLimit]
	}

	// ---- JSON output ----
	if opts.JSON {
		type jsonRow struct {
			Resource   string  `json:"resource"`
			HitsPerSec float64 `json:"hits_per_sec"`
			ErrorRate  float64 `json:"error_rate"`
			AvgLatency float64 `json:"avg_latency_ms"`
		}
		var rows []jsonRow
		for _, k := range keys {
			s := data[k]
			errRate := 0.0
			if s.hits > 0 {
				errRate = s.errors / s.hits * 100
			}
			rows = append(rows, jsonRow{
				Resource:   k,
				HitsPerSec: s.hits,
				ErrorRate:  errRate,
				AvgLatency: s.latency / 1e6, // ns → ms
			})
		}
		return output.RenderJSON(map[string]interface{}{
			"service":    apmEndpointsService,
			"env":        apmEndpointsEnv,
			"from":       fromSecs,
			"to":         toSecs,
			"endpoints":  rows,
			"hits_query": hitsQuery,
		}, opts)
	}

	// ---- empty result ----
	if len(keys) == 0 {
		_, _ = fmt.Fprintf(os.Stdout,
			"No APM endpoint stats found for service=%q env=%q.\n"+
				"Check that tracing is enabled and the service has recent traffic.\n"+
				"Queries used:\n  %s\n  %s\n  %s\n",
			apmEndpointsService, apmEndpointsEnv,
			hitsQuery, errorsQuery, latencyQuery)
		return nil
	}

	// ---- table output ----
	var tableRows [][]string
	var rowStructs []endpointRow
	for _, k := range keys {
		s := data[k]

		errRate := 0.0
		if s.hits > 0 {
			errRate = s.errors / s.hits * 100
		}
		latencyMS := s.latency / 1e6 // nanoseconds → milliseconds

		r := endpointRow{
			Resource:   k,
			HitsPerSec: fmt.Sprintf("%.2f", s.hits),
			ErrorRate:  fmt.Sprintf("%.2f%%", errRate),
			AvgLatency: fmt.Sprintf("%.1f ms", latencyMS),
		}
		rowStructs = append(rowStructs, r)
		tableRows = append(tableRows, []string{r.Resource, r.HitsPerSec, r.ErrorRate, r.AvgLatency})
	}

	cols := []output.ColumnConfig{
		{Name: "Resource", Width: 60},
		{Name: "Hits/s", Width: 12},
		{Name: "Error Rate", Width: 12},
		{Name: "Avg Latency", Width: 14},
	}

	return output.RenderTable(cols, tableRows, rowStructs, opts)
}

// ---- init ----

func init() {
	apmEndpointsCmd.Flags().StringVarP(&apmEndpointsService, "service", "s", "", "APM service name (required)")
	apmEndpointsCmd.Flags().StringVarP(&apmEndpointsEnv, "env", "e", "", "APM environment (e.g. production, staging)")
	apmEndpointsCmd.Flags().StringVar(&apmEndpointsFrom, "from", "1h", "Start of time window (e.g. '1h', '2d', Unix timestamp)")
	apmEndpointsCmd.Flags().StringVar(&apmEndpointsTo, "to", "now", "End of time window ('now' or Unix timestamp)")
	apmEndpointsCmd.Flags().StringVarP(&apmEndpointsResource, "resource", "r", "", "Filter resources by substring match")
	apmEndpointsCmd.Flags().StringVar(&apmEndpointsSort, "sort", "resource", "Sort by: resource, hits, errors, latency")

	_ = apmEndpointsCmd.MarkFlagRequired("service")

	apmCmd.AddCommand(apmEndpointsCmd)
}
