package main

import (
	"context"
	"errors"
	"net"
	"os"

	"grns/internal/api"
)

func formatCLIError(err error) []string {
	if err == nil {
		return nil
	}

	lines := []string{err.Error()}

	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case "unauthorized", "forbidden":
			lines = append(lines, "hint: verify GRNS_API_TOKEN and GRNS_ADMIN_TOKEN configuration.")
		case "resource_exhausted":
			lines = append(lines, "hint: retry shortly or reduce concurrent heavy requests (import/export/search).")
		}
		if apiErr.Code == "" {
			lines = append(lines, "hint: verify GRNS_API_URL points to a grns server.")
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
			"hint: ensure a grns server is running at GRNS_API_URL.",
			"hint: start local server manually with: grns srv",
			"hint: you can increase GRNS_HTTP_TIMEOUT for slower environments.",
		)
		if snapHint := snapStartHint(); snapHint != "" {
			lines = append(lines, snapHint)
		}
		return uniqueLines(lines)
	}

	return uniqueLines(lines)
}

func snapStartHint() string {
	if os.Getenv("SNAP") == "" && os.Getenv("SNAP_NAME") == "" {
		return ""
	}
	return "hint: in snap installs, start the daemon with: snap start grns.daemon"
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
