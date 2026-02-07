package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"grns/internal/api"
	"grns/internal/models"
	storepkg "grns/internal/store"
)

func TestAttachmentLinkCRUDHandlers(t *testing.T) {
	srv := newListTestServer(t)
	seedListTask(t, srv, "gr-at01", "attachment task", 2)

	payload := api.AttachmentCreateLinkRequest{
		Kind:        string(models.AttachmentKindArtifact),
		ExternalURL: "https://example.com/spec.pdf",
		Labels:      []string{"Doc", "doc"},
		MediaType:   "application/pdf",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/gr-at01/attachments/link", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", w.Code, w.Body.String())
	}

	var created models.Attachment
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode attachment: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected attachment id")
	}
	if created.SourceType != string(models.AttachmentSourceExternalURL) {
		t.Fatalf("expected source_type external_url, got %q", created.SourceType)
	}
	if len(created.Labels) != 1 || created.Labels[0] != "doc" {
		t.Fatalf("expected normalized deduped labels [doc], got %#v", created.Labels)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/tasks/gr-at01/attachments", nil)
	w = httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var list []models.Attachment
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode attachment list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(list))
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/attachments/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/attachments/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/attachments/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (%s)", w.Code, w.Body.String())
	}
	var errResp api.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.ErrorCode != ErrCodeAttachmentNotFound {
		t.Fatalf("expected error_code %d, got %d", ErrCodeAttachmentNotFound, errResp.ErrorCode)
	}
}

func TestAttachmentManagedUploadHandler(t *testing.T) {
	srv := newListTestServer(t)
	seedListTask(t, srv, "gr-am01", "managed attachment task", 2)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("kind", string(models.AttachmentKindArtifact))
	_ = writer.WriteField("label", "Build")
	_ = writer.WriteField("label", "build")
	part, err := writer.CreateFormFile("content", "artifact.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("hello attachment world")); err != nil {
		t.Fatalf("write form content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/gr-am01/attachments", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", w.Code, w.Body.String())
	}

	var created models.Attachment
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode attachment: %v", err)
	}
	if created.SourceType != string(models.AttachmentSourceManagedBlob) {
		t.Fatalf("expected source_type managed_blob, got %q", created.SourceType)
	}
	if created.BlobID == "" {
		t.Fatal("expected blob_id to be set")
	}
	if created.MediaType == "" {
		t.Fatal("expected media_type to be set")
	}
	if len(created.Labels) != 1 || created.Labels[0] != "build" {
		t.Fatalf("expected labels [build], got %#v", created.Labels)
	}

	attachmentStore, ok := any(srv.store).(storepkg.AttachmentStore)
	if !ok {
		t.Fatal("expected store to implement AttachmentStore")
	}
	blob, err := attachmentStore.GetBlob(context.Background(), created.BlobID)
	if err != nil {
		t.Fatalf("get blob: %v", err)
	}
	if blob == nil {
		t.Fatal("expected blob row for managed upload")
	}
}

func TestAttachmentContentNotImplemented(t *testing.T) {
	srv := newListTestServer(t)
	seedListTask(t, srv, "gr-ac01", "content task", 2)

	payload := api.AttachmentCreateLinkRequest{
		Kind:        string(models.AttachmentKindArtifact),
		ExternalURL: "https://example.com/a",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/gr-ac01/attachments/link", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", w.Code, w.Body.String())
	}
	var created models.Attachment
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode attachment: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/attachments/"+created.ID+"/content", nil)
	w = httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d (%s)", w.Code, w.Body.String())
	}

	var errResp api.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Code != "not_implemented" {
		t.Fatalf("expected not_implemented code, got %q", errResp.Code)
	}
	if errResp.ErrorCode != ErrCodeNotImplemented {
		t.Fatalf("expected error_code %d, got %d", ErrCodeNotImplemented, errResp.ErrorCode)
	}
}
