package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestAttachmentContentManaged(t *testing.T) {
	srv := newListTestServer(t)
	seedListTask(t, srv, "gr-ac01", "content task", 2)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("kind", string(models.AttachmentKindArtifact))
	part, err := writer.CreateFormFile("content", "artifact.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("hello attachment content")); err != nil {
		t.Fatalf("write form content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/gr-ac01/attachments", body)
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

	req = httptest.NewRequest(http.MethodGet, "/v1/attachments/"+created.ID+"/content", nil)
	w = httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "hello attachment content" {
		t.Fatalf("unexpected content body %q", got)
	}
}

func TestAttachmentContentNonManagedRejected(t *testing.T) {
	srv := newListTestServer(t)
	seedListTask(t, srv, "gr-ac02", "content task", 2)

	payload := api.AttachmentCreateLinkRequest{
		Kind:        string(models.AttachmentKindArtifact),
		ExternalURL: "https://example.com/a",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/gr-ac02/attachments/link", bytes.NewReader(body))
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
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (%s)", w.Code, w.Body.String())
	}

	var errResp api.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.ErrorCode != ErrCodeInvalidArgument {
		t.Fatalf("expected error_code %d, got %d", ErrCodeInvalidArgument, errResp.ErrorCode)
	}
}

func TestAttachmentBlobGCEndToEnd(t *testing.T) {
	srv := newListTestServer(t)
	seedListTask(t, srv, "gr-gc01", "gc task", 2)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("kind", string(models.AttachmentKindArtifact))
	part, err := writer.CreateFormFile("content", "artifact.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("gc me")); err != nil {
		t.Fatalf("write form content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/gr-gc01/attachments", body)
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

	req = httptest.NewRequest(http.MethodDelete, "/v1/attachments/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	dryReq, err := json.Marshal(api.BlobGCRequest{DryRun: true})
	if err != nil {
		t.Fatalf("marshal dry-run request: %v", err)
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/admin/gc-blobs", bytes.NewReader(dryReq))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	var dryResp api.BlobGCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &dryResp); err != nil {
		t.Fatalf("decode dry-run response: %v", err)
	}
	if dryResp.CandidateCount < 1 || !dryResp.DryRun {
		t.Fatalf("unexpected dry-run response: %#v", dryResp)
	}

	applyReq, err := json.Marshal(api.BlobGCRequest{DryRun: false})
	if err != nil {
		t.Fatalf("marshal apply request: %v", err)
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/admin/gc-blobs", bytes.NewReader(applyReq))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Confirm", "true")
	w = httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	var applyResp api.BlobGCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &applyResp); err != nil {
		t.Fatalf("decode apply response: %v", err)
	}
	if applyResp.DeletedCount < 1 || applyResp.DryRun {
		t.Fatalf("unexpected apply response: %#v", applyResp)
	}
}

func TestHandleCreateTaskAttachment_RequestTooLarge_ReturnsStructured1002(t *testing.T) {
	srv := newListTestServer(t)
	seedListTask(t, srv, "gr-lg11", "large upload", 2)

	boundary := "grns-boundary"
	header := fmt.Sprintf("--%s\r\nContent-Disposition: form-data; name=\"kind\"\r\n\r\nartifact\r\n--%s\r\nContent-Disposition: form-data; name=\"content\"; filename=\"big.bin\"\r\nContent-Type: application/octet-stream\r\n\r\n", boundary, boundary)
	trailer := fmt.Sprintf("\r\n--%s--\r\n", boundary)
	payload := io.MultiReader(
		strings.NewReader(header),
		io.LimitReader(zeroReader{}, defaultAttachmentUploadMaxBody+1),
		strings.NewReader(trailer),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/gr-lg11/attachments", payload)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (%s)", w.Code, w.Body.String())
	}

	var errResp api.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.ErrorCode != ErrCodeRequestTooLarge {
		t.Fatalf("expected error_code %d, got %d", ErrCodeRequestTooLarge, errResp.ErrorCode)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
