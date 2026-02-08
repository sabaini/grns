package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
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

func TestClientAttachmentMethods(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects/gr/tasks/gr-aa11/attachments":
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"bad multipart"}`))
				return
			}
			if got := r.FormValue("kind"); got != "artifact" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"missing kind"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"at-a110","task_id":"gr-aa11","kind":"artifact","source_type":"managed_blob","blob_id":"bl-b001"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects/gr/tasks/gr-aa11/attachments/link":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"at-a111","task_id":"gr-aa11","kind":"artifact","source_type":"external_url","external_url":"https://example.com/a"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/projects/gr/tasks/gr-aa11/attachments":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[ {"id":"at-a111","task_id":"gr-aa11","kind":"artifact","source_type":"external_url","external_url":"https://example.com/a"} ]`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/projects/gr/attachments/at-a111":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"at-a111","task_id":"gr-aa11","kind":"artifact","source_type":"external_url","external_url":"https://example.com/a"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/projects/gr/attachments/at-a111/content":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("hello"))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/projects/gr/attachments/at-a111":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"at-a111"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/admin/gc-blobs":
			if r.Header.Get("X-Confirm") != "true" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"missing confirm"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"candidate_count":1,"deleted_count":1,"failed_count":0,"reclaimed_bytes":5,"dry_run":false}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL)

	managed, err := client.CreateTaskAttachment(context.Background(), "gr-aa11", AttachmentUploadRequest{Kind: "artifact", Filename: "a.txt"}, strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("CreateTaskAttachment: %v", err)
	}
	if managed.ID != "at-a110" {
		t.Fatalf("unexpected managed attachment id: %q", managed.ID)
	}

	created, err := client.CreateTaskAttachmentLink(context.Background(), "gr-aa11", AttachmentCreateLinkRequest{Kind: "artifact", ExternalURL: "https://example.com/a"})
	if err != nil {
		t.Fatalf("CreateTaskAttachmentLink: %v", err)
	}
	if created.ID != "at-a111" {
		t.Fatalf("unexpected attachment id: %q", created.ID)
	}

	list, err := client.ListTaskAttachments(context.Background(), "gr-aa11")
	if err != nil {
		t.Fatalf("ListTaskAttachments: %v", err)
	}
	if len(list) != 1 || list[0].ID != "at-a111" {
		t.Fatalf("unexpected attachment list: %#v", list)
	}

	got, err := client.GetAttachment(context.Background(), "at-a111")
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}
	if got.ID != "at-a111" {
		t.Fatalf("unexpected attachment: %#v", got)
	}

	content := &bytes.Buffer{}
	if err := client.GetAttachmentContent(context.Background(), "at-a111", content); err != nil {
		t.Fatalf("GetAttachmentContent: %v", err)
	}
	if content.String() != "hello" {
		t.Fatalf("unexpected attachment content %q", content.String())
	}

	gcResp, err := client.AdminGCBlobs(context.Background(), BlobGCRequest{DryRun: false, BatchSize: 50}, true)
	if err != nil {
		t.Fatalf("AdminGCBlobs: %v", err)
	}
	if gcResp.DeletedCount != 1 || gcResp.ReclaimedBytes != 5 {
		t.Fatalf("unexpected gc response: %#v", gcResp)
	}

	deleted, err := client.DeleteAttachment(context.Background(), "at-a111")
	if err != nil {
		t.Fatalf("DeleteAttachment: %v", err)
	}
	if deleted["id"] != "at-a111" {
		t.Fatalf("unexpected delete response: %#v", deleted)
	}
}

func TestClientRetryPolicy_TransportErrors(t *testing.T) {
	origBase := retryBaseDelay
	origMax := retryMaxDelay
	retryBaseDelay = time.Millisecond
	retryMaxDelay = time.Millisecond
	t.Cleanup(func() {
		retryBaseDelay = origBase
		retryMaxDelay = origMax
	})

	t.Run("retries idempotent get on transport failure", func(t *testing.T) {
		var attempts int32
		client := NewClient("http://example.invalid")
		client.http = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET request, got %s", r.Method)
			}
			try := atomic.AddInt32(&attempts, 1)
			if try <= 2 {
				return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"project_prefix":"gr","schema_version":6,"task_counts":{},"total_tasks":0}`)),
			}, nil
		})}

		_, err := client.GetInfo(context.Background())
		if err != nil {
			t.Fatalf("GetInfo after transport retries: %v", err)
		}
		if got := atomic.LoadInt32(&attempts); got != 3 {
			t.Fatalf("expected 3 attempts (2 retries), got %d", got)
		}
	})

	t.Run("does not retry non idempotent post on transport failure", func(t *testing.T) {
		var attempts int32
		client := NewClient("http://example.invalid")
		client.http = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST request, got %s", r.Method)
			}
			atomic.AddInt32(&attempts, 1)
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
		})}

		_, err := client.CreateTask(context.Background(), TaskCreateRequest{Title: "x"})
		if err == nil {
			t.Fatal("expected CreateTask transport error")
		}
		if got := atomic.LoadInt32(&attempts); got != 1 {
			t.Fatalf("expected one POST attempt, got %d", got)
		}
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
