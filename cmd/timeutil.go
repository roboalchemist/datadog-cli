package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parseTime parses a time string and returns milliseconds since Unix epoch.
//
// Supported formats:
//   - "now"                     → current time
//   - Relative: "15m", "1h", "2d", "1w"  → subtracted from now
//   - ISO-8601: "2024-01-15T10:30:00Z"
//   - Unix timestamp (seconds or ms, auto-detected)
func parseTime(s string) (int64, error) {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)

	// "now"
	if lower == "now" {
		return time.Now().UnixMilli(), nil
	}

	// Relative time: digit(s) + unit suffix
	// Supported units: s, m, h, d, w
	if len(lower) >= 2 {
		unit := lower[len(lower)-1]
		numStr := lower[:len(lower)-1]
		if n, err := strconv.ParseInt(numStr, 10, 64); err == nil {
			var multiplier int64
			switch unit {
			case 's':
				multiplier = 1
			case 'm':
				multiplier = 60
			case 'h':
				multiplier = 3600
			case 'd':
				multiplier = 86400
			case 'w':
				multiplier = 604800
			}
			if multiplier > 0 {
				offsetSecs := n * multiplier
				return time.Now().Add(-time.Duration(offsetSecs) * time.Second).UnixMilli(), nil
			}
		}
	}

	// Unix timestamp (auto-detect ms vs s)
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		// If < 32503680000 it's seconds (year ~3000), otherwise treat as ms
		if n < 32503680000 {
			return n * 1000, nil
		}
		return n, nil
	}

	// ISO-8601 / common datetime formats
	formats := []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, fmt := range formats {
		if t, err := time.Parse(fmt, s); err == nil {
			if t.Location() == time.UTC || t.Location().String() == "" {
				t = time.Date(t.Year(), t.Month(), t.Day(),
					t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
			}
			return t.UnixMilli(), nil
		}
	}

	return 0, fmt.Errorf(
		"invalid time format %q\n"+
			"Supported: 'now', relative (15m, 1h, 2d, 1w), ISO-8601, or Unix timestamp",
		s,
	)
}

