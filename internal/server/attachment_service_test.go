package server

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grns/internal/blobstore"
	"grns/internal/models"
	"grns/internal/store"
)

func TestCreateManagedAttachment_RejectsDeclaredVsSniffedMismatch_WhenEnforced(t *testing.T) {
	svc, st := newAttachmentServiceForTest(t)
	ctx := context.Background()
	now := time.Now().UTC()

	task := &models.Task{ID: "gr-mm11", Title: "Mismatch target", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create task: %v", err)
	}

	blob, err := st.UpsertBlob(ctx, &models.Blob{
		ID:             "bl-mm11",
		SHA256:         strings.Repeat("a", 64),
		SizeBytes:      5,
		StorageBackend: "local_cas",
		BlobKey:        "sha256/aa/aa/" + strings.Repeat("a", 64),
		CreatedAt:      now,
	})
	if err != nil {
		t.Fatalf("upsert blob: %v", err)
	}

	t.Setenv(attachmentRejectMismatchEnvKey, "true")
	_, err = svc.CreateManagedAttachment(ctx, task.ID, CreateManagedAttachmentInput{
		Kind:              string(models.AttachmentKindArtifact),
		BlobID:            blob.ID,
		DeclaredMediaType: "text/plain",
		SniffedMediaType:  "application/pdf",
	})
	if err == nil {
		t.Fatal("expected media type mismatch error")
	}
	if httpStatusFromError(err) != 400 {
		t.Fatalf("expected HTTP 400, got %d (%v)", httpStatusFromError(err), err)
	}

	var apiErr apiError
	if !asAPIError(err, &apiErr) {
		t.Fatalf("expected apiError, got %T", err)
	}
	if apiErr.errCode != ErrCodeInvalidArgument {
		t.Fatalf("expected error_code %d, got %d", ErrCodeInvalidArgument, apiErr.errCode)
	}
	if !strings.Contains(err.Error(), "declared media_type does not match content type") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCreateManagedAttachmentFromReader_ValidatesBeforeWritingBlob(t *testing.T) {
	svc, st := newAttachmentServiceForTest(t)
	ctx := context.Background()
	now := time.Now().UTC()

	task := &models.Task{ID: "gr-mb11", Title: "Blob ordering target", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create task: %v", err)
	}

	_, err := svc.CreateManagedAttachmentFromReader(ctx, task.ID, CreateManagedAttachmentInput{
		Kind:              "invalid-kind",
		DeclaredMediaType: "text/plain",
		SniffedMediaType:  "text/plain",
	}, strings.NewReader("payload"))
	if err == nil {
		t.Fatal("expected invalid kind error")
	}
	if httpStatusFromError(err) != 400 {
		t.Fatalf("expected HTTP 400, got %d (%v)", httpStatusFromError(err), err)
	}

	blobs, err := st.ListUnreferencedBlobs(ctx, 0)
	if err != nil {
		t.Fatalf("list unreferenced blobs: %v", err)
	}
	if len(blobs) != 0 {
		t.Fatalf("expected no blob writes on validation failure, got %d blobs", len(blobs))
	}
}

func TestCreateLinkAttachment_RejectsInvalidSourceCombinations(t *testing.T) {
	svc, st := newAttachmentServiceForTest(t)
	ctx := context.Background()
	now := time.Now().UTC()

	task := &models.Task{ID: "gr-lk11", Title: "Link target", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create task: %v", err)
	}

	tests := []struct {
		name     string
		input    CreateLinkAttachmentInput
		wantCode int
	}{
		{
			name: "both external_url and repo_path",
			input: CreateLinkAttachmentInput{
				Kind:        string(models.AttachmentKindArtifact),
				ExternalURL: "https://example.com/a",
				RepoPath:    "docs/spec.md",
			},
			wantCode: ErrCodeMissingRequired,
		},
		{
			name: "neither source provided",
			input: CreateLinkAttachmentInput{
				Kind: string(models.AttachmentKindArtifact),
			},
			wantCode: ErrCodeMissingRequired,
		},
		{
			name: "invalid external url scheme",
			input: CreateLinkAttachmentInput{
				Kind:        string(models.AttachmentKindArtifact),
				ExternalURL: "ftp://example.com/a",
			},
			wantCode: ErrCodeInvalidArgument,
		},
		{
			name: "repo path escapes workspace",
			input: CreateLinkAttachmentInput{
				Kind:     string(models.AttachmentKindArtifact),
				RepoPath: "../secret.txt",
			},
			wantCode: ErrCodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateLinkAttachment(ctx, task.ID, tt.input)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if httpStatusFromError(err) != 400 {
				t.Fatalf("expected HTTP 400, got %d (%v)", httpStatusFromError(err), err)
			}
			var apiErr apiError
			if !asAPIError(err, &apiErr) {
				t.Fatalf("expected apiError, got %T", err)
			}
			if apiErr.errCode != tt.wantCode {
				t.Fatalf("expected error_code %d, got %d", tt.wantCode, apiErr.errCode)
			}
		})
	}
}

func TestCreateManagedAttachmentFromReader_RejectsExpiredAttachment(t *testing.T) {
	svc, st := newAttachmentServiceForTest(t)
	ctx := context.Background()
	now := time.Now().UTC()

	task := &models.Task{ID: "gr-ex11", Title: "Expiry target", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create task: %v", err)
	}

	expiresAt := now.Add(-time.Hour)
	_, err := svc.CreateManagedAttachmentFromReader(ctx, task.ID, CreateManagedAttachmentInput{
		Kind:      string(models.AttachmentKindArtifact),
		ExpiresAt: &expiresAt,
	}, strings.NewReader("payload"))
	if err == nil {
		t.Fatal("expected expires_at validation error")
	}
	if httpStatusFromError(err) != 400 {
		t.Fatalf("expected HTTP 400, got %d (%v)", httpStatusFromError(err), err)
	}
	var apiErr apiError
	if !asAPIError(err, &apiErr) {
		t.Fatalf("expected apiError, got %T", err)
	}
	if apiErr.errCode != ErrCodeInvalidArgument {
		t.Fatalf("expected error_code %d, got %d", ErrCodeInvalidArgument, apiErr.errCode)
	}
}

func TestCreateLinkAttachment_RejectsExpiredAttachment(t *testing.T) {
	svc, st := newAttachmentServiceForTest(t)
	ctx := context.Background()
	now := time.Now().UTC()

	task := &models.Task{ID: "gr-ex22", Title: "Expiry target", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create task: %v", err)
	}

	expiresAt := now.Add(-time.Hour)
	_, err := svc.CreateLinkAttachment(ctx, task.ID, CreateLinkAttachmentInput{
		Kind:        string(models.AttachmentKindArtifact),
		ExternalURL: "https://example.com/a",
		ExpiresAt:   &expiresAt,
	})
	if err == nil {
		t.Fatal("expected expires_at validation error")
	}
	if httpStatusFromError(err) != 400 {
		t.Fatalf("expected HTTP 400, got %d (%v)", httpStatusFromError(err), err)
	}
	var apiErr apiError
	if !asAPIError(err, &apiErr) {
		t.Fatalf("expected apiError, got %T", err)
	}
	if apiErr.errCode != ErrCodeInvalidArgument {
		t.Fatalf("expected error_code %d, got %d", ErrCodeInvalidArgument, apiErr.errCode)
	}
}

func TestAttachmentServiceGCBlobs_TerminatesWhenDeletesFail(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "attachment_service_gc_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	now := time.Now().UTC()
	if _, err := st.UpsertBlob(context.Background(), &models.Blob{
		ID:             "bl-gc11",
		SHA256:         strings.Repeat("b", 64),
		SizeBytes:      3,
		StorageBackend: "local_cas",
		BlobKey:        "sha256/bb/bb/" + strings.Repeat("b", 64),
		CreatedAt:      now,
	}); err != nil {
		t.Fatalf("upsert blob: %v", err)
	}

	svc := NewAttachmentService(st, st, failingDeleteBlobStore{})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	result, err := svc.GCBlobs(ctx, 1, true)
	if err != nil {
		t.Fatalf("gc blobs: %v", err)
	}
	if result.CandidateCount == 0 {
		t.Fatalf("expected candidates, got %#v", result)
	}
	if result.DeletedCount != 0 {
		t.Fatalf("expected no deletions, got %#v", result)
	}
	if result.FailedCount == 0 {
		t.Fatalf("expected failed deletions, got %#v", result)
	}
}

type failingDeleteBlobStore struct{}

func (failingDeleteBlobStore) Put(context.Context, io.Reader) (blobstore.BlobPutResult, error) {
	return blobstore.BlobPutResult{}, errors.New("not implemented")
}

func (failingDeleteBlobStore) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (failingDeleteBlobStore) Delete(context.Context, string) error {
	return errors.New("delete failed")
}

func newAttachmentServiceForTest(t *testing.T) (*AttachmentService, *store.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "attachment_service_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	cas, err := blobstore.NewLocalCAS(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatalf("open blob store: %v", err)
	}

	return NewAttachmentService(st, st, cas), st
}

func asAPIError(err error, out *apiError) bool {
	if err == nil || out == nil {
		return false
	}
	v, ok := err.(apiError)
	if !ok {
		return false
	}
	*out = v
	return true
}
