// Package timeparse provides human-readable time format parsing.
package timeparse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseError is returned when a time string cannot be parsed.
type ParseError struct {
	Input   string
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("invalid time format: '%s'. %s", e.Input, e.Message)
}

// Parse parses a human-readable time string into a time.Duration.
//
// Supported formats:
//   - "30s" or "30sec" - seconds
//   - "5m" or "5min" - minutes
//   - "2h" or "2hr" - hours
//   - "1d" or "1day" - days
//   - Combined: "1h30m", "2d12h", etc.
//   - Decimals: "1.5h" (90 minutes)
func Parse(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, &ParseError{Input: s, Message: "Time string cannot be empty"}
	}

	pattern := regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(s|sec|seconds?|m|min|minutes?|h|hr|hours?|d|days?)`)
	matches := pattern.FindAllStringSubmatch(s, -1)

	if len(matches) == 0 {
		return 0, &ParseError{
			Input:   s,
			Message: "Use formats like '5m', '2h', '1d', '30s', or combined '1h30m'",
		}
	}

	// Verify entire string was consumed
	consumed := ""
	for _, match := range matches {
		consumed += match[0]
	}
	cleaned := regexp.MustCompile(`\s+`).ReplaceAllString(s, "")
	if consumed != cleaned {
		return 0, &ParseError{
			Input:   s,
			Message: "Contains unrecognized characters.",
		}
	}

	var totalSeconds float64

	for _, match := range matches {
		value, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			return 0, &ParseError{Input: s, Message: fmt.Sprintf("Invalid number: %s", match[1])}
		}

		unit := match[2]
		switch {
		case strings.HasPrefix(unit, "s"):
			totalSeconds += value
		case strings.HasPrefix(unit, "m"):
			totalSeconds += value * 60
		case strings.HasPrefix(unit, "h"):
			totalSeconds += value * 3600
		case strings.HasPrefix(unit, "d"):
			totalSeconds += value * 86400
		}
	}

	if totalSeconds <= 0 {
		return 0, &ParseError{Input: s, Message: "Time must be greater than zero"}
	}

	return time.Duration(totalSeconds * float64(time.Second)), nil
}

// FormatDuration formats a duration into a human-readable string like "5m 30s", "2h 15m".
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}

	totalSeconds := int(d.Seconds())
	days := totalSeconds / 86400
	remainder := totalSeconds % 86400
	hours := remainder / 3600
	remainder = remainder % 3600
	minutes := remainder / 60
	seconds := remainder % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}

	return strings.Join(parts, " ")
}

// LooksLikeTimeSpec returns true if the string looks like it could be a time specification.
// Used to distinguish "5m" (time) from a command name.
func LooksLikeTimeSpec(s string) bool {
	pattern := regexp.MustCompile(`^\d+(?:\.\d+)?\s*(s|sec|seconds?|m|min|minutes?|h|hr|hours?|d|days?)`)
	return pattern.MatchString(strings.TrimSpace(strings.ToLower(s)))
}
