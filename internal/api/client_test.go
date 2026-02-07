package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestClientRetriesIdempotentGetOn5xx(t *testing.T) {
	origBase := retryBaseDelay
	origMax := retryMaxDelay
	retryBaseDelay = time.Millisecond
	retryMaxDelay = time.Millisecond
	t.Cleanup(func() {
		retryBaseDelay = origBase
		retryMaxDelay = origMax
	})

	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		try := atomic.AddInt32(&attempts, 1)
		if try <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"retry me"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"project_prefix":"gr","schema_version":3,"task_counts":{},"total_tasks":0}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.GetInfo(context.Background())
	if err != nil {
		t.Fatalf("GetInfo after retries: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected 3 attempts (2 retries), got %d", got)
	}
}

func TestClientDoesNotRetryNonIdempotentPostOn5xx(t *testing.T) {
	origBase := retryBaseDelay
	origMax := retryMaxDelay
	retryBaseDelay = time.Millisecond
	retryMaxDelay = time.Millisecond
	t.Cleanup(func() {
		retryBaseDelay = origBase
		retryMaxDelay = origMax
	})

	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"busy"}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.CreateTask(context.Background(), TaskCreateRequest{Title: "x"})
	if err == nil {
		t.Fatal("expected create to fail")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("expected one attempt for non-idempotent POST, got %d", got)
	}
}

func TestClientRetriesThenReturnsServerError(t *testing.T) {
	origBase := retryBaseDelay
	origMax := retryMaxDelay
	retryBaseDelay = time.Millisecond
	retryMaxDelay = time.Millisecond
	t.Cleanup(func() {
		retryBaseDelay = origBase
		retryMaxDelay = origMax
	})

	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream unavailable"}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.GetInfo(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected 3 attempts before failing, got %d", got)
	}
	if want := "upstream unavailable"; !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error to contain %q, got %v", want, err)
	}
}

func TestClientDecodeStructuredErrorCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid id","code":"invalid_argument","error_code":1004}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.GetInfo(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Code != "invalid_argument" {
		t.Fatalf("unexpected code: %q", apiErr.Code)
	}
	if apiErr.ErrorCode != 1004 {
		t.Fatalf("unexpected error_code: %d", apiErr.ErrorCode)
	}
	if apiErr.Status != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d", apiErr.Status)
	}
}
