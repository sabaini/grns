package main

import (
	"context"
	"errors"
	"fmt"
	"net"

	"grns/internal/api"
)

type startupDiagnosticsError struct {
	apiURL  string
	logPath string
	cause   error
}

func (e *startupDiagnosticsError) Error() string {
	if e == nil {
		return ""
	}
	if e.logPath != "" {
		return fmt.Sprintf("failed to auto-start server for %s (see %s): %v", e.apiURL, e.logPath, e.cause)
	}
	return fmt.Sprintf("failed to auto-start server for %s: %v", e.apiURL, e.cause)
}

func (e *startupDiagnosticsError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func formatCLIError(err error) []string {
	if err == nil {
		return nil
	}

	lines := []string{err.Error()}

	var startupErr *startupDiagnosticsError
	if errors.As(err, &startupErr) {
		lines = append(lines,
			"hint: verify GRNS_API_URL points to a grns server (or allow auto-spawn).",
		)
		if startupErr.logPath != "" {
			lines = append(lines, fmt.Sprintf("hint: inspect server log: %s", startupErr.logPath))
		}
		return uniqueLines(lines)
	}

	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case "unauthorized", "forbidden":
			lines = append(lines, "hint: verify GRNS_API_TOKEN and GRNS_ADMIN_TOKEN configuration.")
		case "resource_exhausted":
			lines = append(lines, "hint: retry shortly or reduce concurrent heavy requests (import/export/search).")
		}
		if apiErr.Status >= 500 {
			lines = append(lines, "hint: server returned an internal error; check server logs for details.")
		}
		return uniqueLines(lines)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		lines = append(lines, "hint: request timed out; check server health or increase GRNS_HTTP_TIMEOUT.")
		return uniqueLines(lines)
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		lines = append(lines,
			"hint: network error contacting API; verify GRNS_API_URL and server availability.",
			"hint: you can increase GRNS_HTTP_TIMEOUT for slower environments.",
		)
		return uniqueLines(lines)
	}

	return uniqueLines(lines)
}

func uniqueLines(lines []string) []string {
	seen := make(map[string]struct{}, len(lines))
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		out = append(out, line)
	}
	return out
}
