package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"grns/internal/models"
)

func TestCreateGetListDeleteAttachment_RoundTrip(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	task := &models.Task{ID: "gr-at11", Title: "Attachment task", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create task: %v", err)
	}

	blob, err := st.UpsertBlob(ctx, &models.Blob{
		ID:             "bl-a111",
		SHA256:         strings.Repeat("a", 64),
		SizeBytes:      5,
		StorageBackend: "local_cas",
		BlobKey:        "sha256/aa/aa/" + strings.Repeat("a", 64),
		CreatedAt:      now,
	})
	if err != nil {
		t.Fatalf("upsert blob: %v", err)
	}

	managed := &models.Attachment{
		ID:              "at-a111",
		TaskID:          task.ID,
		Kind:            string(models.AttachmentKindArtifact),
		SourceType:      string(models.AttachmentSourceManagedBlob),
		Filename:        "artifact.txt",
		MediaType:       "text/plain",
		MediaTypeSource: string(models.MediaTypeSourceDeclared),
		BlobID:          blob.ID,
		Labels:          []string{"Doc", "doc"},
		Meta:            map[string]any{"k": "v"},
		CreatedAt:       now.Add(-2 * time.Second),
		UpdatedAt:       now.Add(-2 * time.Second),
	}
	if err := st.CreateAttachment(ctx, managed); err != nil {
		t.Fatalf("create managed attachment: %v", err)
	}

	link := &models.Attachment{
		ID:              "at-a112",
		TaskID:          task.ID,
		Kind:            string(models.AttachmentKindSpec),
		SourceType:      string(models.AttachmentSourceExternalURL),
		ExternalURL:     "https://example.com/spec.pdf",
		MediaType:       "application/pdf",
		MediaTypeSource: string(models.MediaTypeSourceDeclared),
		Labels:          []string{"Ref"},
		Meta:            map[string]any{"source": "ext"},
		CreatedAt:       now.Add(-1 * time.Second),
		UpdatedAt:       now.Add(-1 * time.Second),
	}
	if err := st.CreateAttachment(ctx, link); err != nil {
		t.Fatalf("create link attachment: %v", err)
	}

	gotManaged, err := st.GetAttachment(ctx, managed.ID)
	if err != nil {
		t.Fatalf("get managed attachment: %v", err)
	}
	if gotManaged == nil {
		t.Fatal("expected managed attachment")
	}
	if gotManaged.SourceType != string(models.AttachmentSourceManagedBlob) {
		t.Fatalf("expected managed source_type, got %q", gotManaged.SourceType)
	}
	if len(gotManaged.Labels) != 1 || gotManaged.Labels[0] != "doc" {
		t.Fatalf("expected normalized labels [doc], got %v", gotManaged.Labels)
	}
	if gotManaged.Meta["k"] != "v" {
		t.Fatalf("expected meta roundtrip, got %v", gotManaged.Meta)
	}

	list, err := st.ListAttachmentsByTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("list attachments by task: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(list))
	}
	if list[0].ID != link.ID || list[1].ID != managed.ID {
		t.Fatalf("expected created_at desc order [%s %s], got [%s %s]", link.ID, managed.ID, list[0].ID, list[1].ID)
	}

	if err := st.DeleteAttachment(ctx, managed.ID); err != nil {
		t.Fatalf("delete managed attachment: %v", err)
	}

	deleted, err := st.GetAttachment(ctx, managed.ID)
	if err != nil {
		t.Fatalf("get deleted attachment: %v", err)
	}
	if deleted != nil {
		t.Fatalf("expected nil attachment after delete, got %#v", deleted)
	}

	remaining, err := st.ListAttachmentsByTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("list remaining attachments: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != link.ID {
		t.Fatalf("unexpected remaining attachments: %#v", remaining)
	}
}

func TestUpsertBlob_DedupesBySHAAndPreservesCanonicalRow(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	sha := strings.Repeat("b", 64)

	first, err := st.UpsertBlob(ctx, &models.Blob{
		ID:             "bl-b111",
		SHA256:         sha,
		SizeBytes:      10,
		StorageBackend: "local_cas",
		BlobKey:        "sha256/bb/bb/" + sha,
		CreatedAt:      now,
	})
	if err != nil {
		t.Fatalf("upsert first blob: %v", err)
	}

	second, err := st.UpsertBlob(ctx, &models.Blob{
		ID:             "bl-b222",
		SHA256:         sha,
		SizeBytes:      999,
		StorageBackend: "local_cas",
		BlobKey:        "sha256/bb/bb/changed",
		CreatedAt:      now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("upsert second blob: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected same canonical blob id for duplicate sha; first=%s second=%s", first.ID, second.ID)
	}
	if second.SizeBytes != first.SizeBytes {
		t.Fatalf("expected canonical row to preserve original size_bytes=%d, got %d", first.SizeBytes, second.SizeBytes)
	}
	if second.BlobKey != first.BlobKey {
		t.Fatalf("expected canonical row to preserve original blob_key=%q, got %q", first.BlobKey, second.BlobKey)
	}
}

func TestCreateManagedAttachmentWithBlob_RollsBackBlobOnAttachmentInsertError(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	blobSHA := strings.Repeat("f", 64)
	_, err := st.CreateManagedAttachmentWithBlob(ctx, &models.Blob{
		SHA256:         blobSHA,
		SizeBytes:      10,
		StorageBackend: "local_cas",
		BlobKey:        "sha256/ff/ff/" + blobSHA,
		CreatedAt:      now,
	}, &models.Attachment{
		ID:              "at-z111",
		TaskID:          "gr-miss",
		Kind:            string(models.AttachmentKindArtifact),
		SourceType:      string(models.AttachmentSourceManagedBlob),
		MediaTypeSource: string(models.MediaTypeSourceUnknown),
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err == nil {
		t.Fatal("expected create managed attachment to fail for missing task")
	}

	blob, err := st.GetBlobBySHA256(ctx, blobSHA)
	if err != nil {
		t.Fatalf("get blob by sha after rollback: %v", err)
	}
	if blob != nil {
		t.Fatalf("expected blob metadata rollback on attachment insert error, got %#v", blob)
	}
}

func TestListUnreferencedBlobs_OnlyReturnsUnattachedBlobs(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	task := &models.Task{ID: "gr-bu11", Title: "Blob task", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create task: %v", err)
	}

	attached, err := st.UpsertBlob(ctx, &models.Blob{ID: "bl-c111", SHA256: strings.Repeat("c", 64), SizeBytes: 10, StorageBackend: "local_cas", BlobKey: "sha256/cc/cc/" + strings.Repeat("c", 64), CreatedAt: now})
	if err != nil {
		t.Fatalf("upsert attached blob: %v", err)
	}
	orphanOne, err := st.UpsertBlob(ctx, &models.Blob{ID: "bl-d111", SHA256: strings.Repeat("d", 64), SizeBytes: 11, StorageBackend: "local_cas", BlobKey: "sha256/dd/dd/" + strings.Repeat("d", 64), CreatedAt: now})
	if err != nil {
		t.Fatalf("upsert orphan blob 1: %v", err)
	}
	orphanTwo, err := st.UpsertBlob(ctx, &models.Blob{ID: "bl-e111", SHA256: strings.Repeat("e", 64), SizeBytes: 12, StorageBackend: "local_cas", BlobKey: "sha256/ee/ee/" + strings.Repeat("e", 64), CreatedAt: now})
	if err != nil {
		t.Fatalf("upsert orphan blob 2: %v", err)
	}

	if err := st.CreateAttachment(ctx, &models.Attachment{
		ID:              "at-b111",
		TaskID:          task.ID,
		Kind:            string(models.AttachmentKindArtifact),
		SourceType:      string(models.AttachmentSourceManagedBlob),
		MediaTypeSource: string(models.MediaTypeSourceUnknown),
		BlobID:          attached.ID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("create attached attachment: %v", err)
	}

	unreferenced, err := st.ListUnreferencedBlobs(ctx, 0)
	if err != nil {
		t.Fatalf("list unreferenced blobs: %v", err)
	}
	ids := make(map[string]struct{}, len(unreferenced))
	for _, blob := range unreferenced {
		ids[blob.ID] = struct{}{}
	}
	if _, ok := ids[attached.ID]; ok {
		t.Fatalf("attached blob %s must not be returned as unreferenced", attached.ID)
	}
	if _, ok := ids[orphanOne.ID]; !ok {
		t.Fatalf("expected orphan blob %s in result set", orphanOne.ID)
	}
	if _, ok := ids[orphanTwo.ID]; !ok {
		t.Fatalf("expected orphan blob %s in result set", orphanTwo.ID)
	}

	limited, err := st.ListUnreferencedBlobs(ctx, 1)
	if err != nil {
		t.Fatalf("list unreferenced blobs with limit: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected limit=1 to return exactly one blob, got %d", len(limited))
	}
	if limited[0].ID == attached.ID {
		t.Fatalf("attached blob %s must not appear in limited unreferenced results", attached.ID)
	}
}
