package main

import (
	"errors"
	"testing"

	"grns/internal/api"
)

func TestFormatCLIError_StartupDiagnostics(t *testing.T) {
	err := &startupDiagnosticsError{
		apiURL:  "http://127.0.0.1:7333",
		logPath: "/tmp/grns/server.log",
		cause:   errors.New("server did not start"),
	}

	lines := formatCLIError(err)
	if len(lines) < 3 {
		t.Fatalf("expected startup guidance lines, got %v", lines)
	}
	if lines[0] == "" {
		t.Fatalf("expected primary error message, got %v", lines)
	}
	if !containsLine(lines, "hint: inspect server log: /tmp/grns/server.log") {
		t.Fatalf("expected log-path guidance, got %v", lines)
	}
}

func TestFormatCLIError_APIAuthGuidance(t *testing.T) {
	err := &api.APIError{Status: 401, Code: "unauthorized", Message: "unauthorized"}
	lines := formatCLIError(err)
	if !containsLine(lines, "hint: verify GRNS_API_TOKEN and GRNS_ADMIN_TOKEN configuration.") {
		t.Fatalf("expected auth guidance, got %v", lines)
	}
}

func TestFormatCLIError_APIInternalGuidance(t *testing.T) {
	err := &api.APIError{Status: 500, Code: "internal", Message: "internal error"}
	lines := formatCLIError(err)
	if !containsLine(lines, "hint: server returned an internal error; check server logs for details.") {
		t.Fatalf("expected internal-error guidance, got %v", lines)
	}
}

func containsLine(lines []string, expected string) bool {
	for _, line := range lines {
		if line == expected {
			return true
		}
	}
	return false
}
