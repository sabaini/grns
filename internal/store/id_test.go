package store

import (
	"testing"
)

func TestGenerateID(t *testing.T) {
	t.Run("valid prefix", func(t *testing.T) {
		id, err := GenerateID("gr", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(id) != 7 { // "gr-" + 4 chars
			t.Fatalf("expected length 7, got %d: %s", len(id), id)
		}
		if id[:3] != "gr-" {
			t.Fatalf("expected prefix gr-, got %s", id[:3])
		}
	})

	t.Run("empty prefix", func(t *testing.T) {
		_, err := GenerateID("", nil)
		if err == nil {
			t.Fatal("expected error for empty prefix")
		}
	})

	t.Run("retries on collision", func(t *testing.T) {
		calls := 0
		exists := func(id string) (bool, error) {
			calls++
			return calls < 3, nil // first 2 calls collide
		}
		id, err := GenerateID("gr", exists)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id == "" {
			t.Fatal("expected non-empty id")
		}
		if calls != 3 {
			t.Fatalf("expected 3 calls, got %d", calls)
		}
	})

	t.Run("gives up after max attempts", func(t *testing.T) {
		exists := func(id string) (bool, error) {
			return true, nil // always collide
		}
		_, err := GenerateID("gr", exists)
		if err == nil {
			t.Fatal("expected error after max attempts")
		}
	})
}

func TestGenerateAttachmentAndBlobID(t *testing.T) {
	attachmentID, err := GenerateAttachmentID(nil)
	if err != nil {
		t.Fatalf("generate attachment id: %v", err)
	}
	if len(attachmentID) != 7 || attachmentID[:3] != "at-" {
		t.Fatalf("expected attachment id with at- prefix, got %q", attachmentID)
	}

	blobID, err := GenerateBlobID(nil)
	if err != nil {
		t.Fatalf("generate blob id: %v", err)
	}
	if len(blobID) != 7 || blobID[:3] != "bl-" {
		t.Fatalf("expected blob id with bl- prefix, got %q", blobID)
	}
}
