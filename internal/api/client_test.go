package api

import (
	"testing"
	"time"
)

func TestHTTPTimeoutFromEnv(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv(httpTimeoutEnvKey, "")
		if got := httpTimeoutFromEnv(); got != defaultHTTPTimeout {
			t.Fatalf("expected default timeout %v, got %v", defaultHTTPTimeout, got)
		}
	})

	t.Run("duration format", func(t *testing.T) {
		t.Setenv(httpTimeoutEnvKey, "45s")
		if got := httpTimeoutFromEnv(); got != 45*time.Second {
			t.Fatalf("expected 45s timeout, got %v", got)
		}
	})

	t.Run("integer seconds", func(t *testing.T) {
		t.Setenv(httpTimeoutEnvKey, "25")
		if got := httpTimeoutFromEnv(); got != 25*time.Second {
			t.Fatalf("expected 25s timeout, got %v", got)
		}
	})

	t.Run("invalid falls back", func(t *testing.T) {
		t.Setenv(httpTimeoutEnvKey, "invalid")
		if got := httpTimeoutFromEnv(); got != defaultHTTPTimeout {
			t.Fatalf("expected default timeout %v, got %v", defaultHTTPTimeout, got)
		}
	})
}
