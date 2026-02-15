package main

import (
	"net"
	"testing"

	"grns/internal/api"
)

func TestFormatCLIError_NetworkGuidance(t *testing.T) {
	err := &net.DNSError{Err: "dial tcp: connection refused", Name: "127.0.0.1", IsTemporary: true}
	lines := formatCLIError(err)
	if !containsLine(lines, "hint: ensure a grns server is running at GRNS_API_URL.") {
		t.Fatalf("expected connectivity guidance, got %v", lines)
	}
	if !containsLine(lines, "hint: start local server manually with: grns srv") {
		t.Fatalf("expected manual-start guidance, got %v", lines)
	}
}

func TestFormatCLIError_APIUnknownServiceGuidance(t *testing.T) {
	err := &api.APIError{Status: 404, Message: "api error: 404 Not Found"}
	lines := formatCLIError(err)
	if !containsLine(lines, "hint: verify GRNS_API_URL points to a grns server.") {
		t.Fatalf("expected api-url guidance, got %v", lines)
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
