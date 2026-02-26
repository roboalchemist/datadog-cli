package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/itchyny/gojq"
	"github.com/olekukonko/tablewriter"
)

// Options controls output formatting.
type Options struct {
	JSON      bool
	Plaintext bool
	NoColor   bool
	JQExpr    string
	Fields    []string
}

// Printer handles formatted output.
type Printer struct {
	opts   Options
	writer io.Writer
}

// NewPrinter creates a new Printer with the given options.
func NewPrinter(opts Options) *Printer {
	if opts.NoColor {
		color.NoColor = true
	}
	return &Printer{
		opts:   opts,
		writer: os.Stdout,
	}
}

// NewPrinterWithWriter creates a new Printer writing to the given writer.
func NewPrinterWithWriter(opts Options, w io.Writer) *Printer {
	if opts.NoColor {
		color.NoColor = true
	}
	return &Printer{
		opts:   opts,
		writer: w,
	}
}

// PrintJSON outputs data as formatted JSON, optionally applying a jq expression.
func (p *Printer) PrintJSON(data interface{}) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}

	if p.opts.JQExpr != "" {
		return p.applyJQ(b)
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		return fmt.Errorf("indenting JSON: %w", err)
	}
	fmt.Fprintln(p.writer, buf.String())
	return nil
}

// PrintTable renders data as a table.
// headers is the column names, rows is the data rows.
func (p *Printer) PrintTable(headers []string, rows [][]string) {
	if p.opts.JSON {
		// Convert to list of maps for JSON output
		result := make([]map[string]string, 0, len(rows))
		for _, row := range rows {
			m := make(map[string]string, len(headers))
			for i, h := range headers {
				if i < len(row) {
					m[h] = row[i]
				}
			}
			result = append(result, m)
		}
		_ = p.PrintJSON(result)
		return
	}

	table := tablewriter.NewWriter(p.writer)
	table.SetHeader(headers)

	if p.opts.Plaintext {
		table.SetBorder(false)
		table.SetColumnSeparator(" ")
		table.SetCenterSeparator(" ")
		table.SetRowSeparator(" ")
		table.SetHeaderLine(false)
		table.SetTablePadding(" ")
		table.SetNoWhiteSpace(true)
	} else {
		table.SetBorder(true)
		table.SetRowLine(false)
		table.SetAutoWrapText(false)
	}

	for _, row := range rows {
		table.Append(row)
	}
	table.Render()
}

// PrintError prints an error message to stderr.
func PrintError(err error) {
	fmt.Fprintf(os.Stderr, "%s %v\n", color.RedString("Error:"), err)
}

// PrintErrorf prints a formatted error message to stderr.
func PrintErrorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("Error:"), fmt.Sprintf(format, args...))
}

// applyJQ applies a jq expression to the given JSON bytes and prints the result.
func (p *Printer) applyJQ(data []byte) error {
	query, err := gojq.Parse(p.opts.JQExpr)
	if err != nil {
		return fmt.Errorf("parsing jq expression %q: %w", p.opts.JQExpr, err)
	}

	var input interface{}
	if err := json.Unmarshal(data, &input); err != nil {
		return fmt.Errorf("parsing JSON for jq: %w", err)
	}

	iter := query.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return fmt.Errorf("jq error: %w", err)
		}
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling jq result: %w", err)
		}
		fmt.Fprintln(p.writer, string(b))
	}
	return nil
}
