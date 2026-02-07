package store

import (
	"testing"
	"time"
)

func TestIntFromEnv(t *testing.T) {
	t.Setenv(maxOpenConnsEnvKey, "")
	if got := intFromEnv(maxOpenConnsEnvKey, 3); got != 3 {
		t.Fatalf("expected default 3, got %d", got)
	}

	t.Setenv(maxOpenConnsEnvKey, "4")
	if got := intFromEnv(maxOpenConnsEnvKey, 3); got != 4 {
		t.Fatalf("expected parsed 4, got %d", got)
	}

	t.Setenv(maxOpenConnsEnvKey, "bad")
	if got := intFromEnv(maxOpenConnsEnvKey, 3); got != 3 {
		t.Fatalf("expected default on invalid value, got %d", got)
	}

	t.Setenv(maxOpenConnsEnvKey, "0")
	if got := intFromEnv(maxOpenConnsEnvKey, 3); got != 3 {
		t.Fatalf("expected default on non-positive value, got %d", got)
	}
}

func TestDurationFromEnv(t *testing.T) {
	t.Setenv(connMaxLifetimeEnvKey, "")
	if got := durationFromEnv(connMaxLifetimeEnvKey, 2*time.Minute); got != 2*time.Minute {
		t.Fatalf("expected default duration, got %v", got)
	}

	t.Setenv(connMaxLifetimeEnvKey, "45s")
	if got := durationFromEnv(connMaxLifetimeEnvKey, 2*time.Minute); got != 45*time.Second {
		t.Fatalf("expected parsed duration, got %v", got)
	}

	t.Setenv(connMaxLifetimeEnvKey, "30")
	if got := durationFromEnv(connMaxLifetimeEnvKey, 2*time.Minute); got != 30*time.Second {
		t.Fatalf("expected seconds duration, got %v", got)
	}

	t.Setenv(connMaxLifetimeEnvKey, "invalid")
	if got := durationFromEnv(connMaxLifetimeEnvKey, 2*time.Minute); got != 2*time.Minute {
		t.Fatalf("expected default on invalid duration, got %v", got)
	}
}
