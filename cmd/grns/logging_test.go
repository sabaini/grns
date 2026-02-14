package main

import (
	"log/slog"
	"strings"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    slog.Level
		wantErr bool
	}{
		{name: "default debug", raw: "", want: slog.LevelDebug},
		{name: "debug", raw: "debug", want: slog.LevelDebug},
		{name: "info", raw: "info", want: slog.LevelInfo},
		{name: "warn", raw: "warn", want: slog.LevelWarn},
		{name: "warning alias", raw: "warning", want: slog.LevelWarn},
		{name: "error", raw: "error", want: slog.LevelError},
		{name: "numeric", raw: "-4", want: slog.LevelDebug},
		{name: "invalid", raw: "verbose", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLogLevel(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parse level: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestSelectedLogLevel(t *testing.T) {
	raw, source := selectedLogLevel("debug", "error", "warn")
	if raw != "debug" || source != "flag" {
		t.Fatalf("expected flag precedence, got raw=%q source=%q", raw, source)
	}

	raw, source = selectedLogLevel("", "warn", "info")
	if raw != "warn" || source != "env" {
		t.Fatalf("expected env fallback, got raw=%q source=%q", raw, source)
	}

	raw, source = selectedLogLevel("", "", "error")
	if raw != "error" || source != "config" {
		t.Fatalf("expected config fallback, got raw=%q source=%q", raw, source)
	}

	raw, source = selectedLogLevel("", "", "")
	if raw != "" || source != "default" {
		t.Fatalf("expected default fallback, got raw=%q source=%q", raw, source)
	}
}

func TestConfigureLoggerForCLI(t *testing.T) {
	t.Run("flag overrides invalid env", func(t *testing.T) {
		t.Setenv(logLevelEnvKey, "invalid")
		warning, err := configureLoggerForCLI("debug", "info")
		if err != nil {
			t.Fatalf("configure logger: %v", err)
		}
		if warning != "" {
			t.Fatalf("expected no warning, got %q", warning)
		}
	})

	t.Run("invalid flag returns error", func(t *testing.T) {
		t.Setenv(logLevelEnvKey, "")
		warning, err := configureLoggerForCLI("verbose", "info")
		if err == nil {
			t.Fatal("expected error")
		}
		if warning != "" {
			t.Fatalf("expected empty warning, got %q", warning)
		}
	})

	t.Run("invalid env returns warning and fallback", func(t *testing.T) {
		t.Setenv(logLevelEnvKey, "verbose")
		warning, err := configureLoggerForCLI("", "info")
		if err != nil {
			t.Fatalf("configure logger: %v", err)
		}
		if !strings.Contains(warning, "defaulting to debug") {
			t.Fatalf("expected fallback warning, got %q", warning)
		}
	})

	t.Run("invalid config returns warning and fallback", func(t *testing.T) {
		t.Setenv(logLevelEnvKey, "")
		warning, err := configureLoggerForCLI("", "verbose")
		if err != nil {
			t.Fatalf("configure logger: %v", err)
		}
		if !strings.Contains(warning, "invalid log_level") {
			t.Fatalf("expected config warning, got %q", warning)
		}
		if !strings.Contains(warning, "defaulting to debug") {
			t.Fatalf("expected debug fallback warning, got %q", warning)
		}
	})
}
