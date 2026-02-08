package blobstore

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func TestLocalCASPutOpenDelete(t *testing.T) {
	cas, err := NewLocalCAS(t.TempDir())
	if err != nil {
		t.Fatalf("new local cas: %v", err)
	}

	first, err := cas.Put(context.Background(), bytes.NewBufferString("hello"))
	if err != nil {
		t.Fatalf("put first: %v", err)
	}
	if first.SHA256 == "" || first.BlobKey == "" {
		t.Fatalf("unexpected put result: %#v", first)
	}

	second, err := cas.Put(context.Background(), bytes.NewBufferString("hello"))
	if err != nil {
		t.Fatalf("put second: %v", err)
	}
	if first.BlobKey != second.BlobKey || first.SHA256 != second.SHA256 {
		t.Fatalf("expected dedupe keys/digests to match: first=%#v second=%#v", first, second)
	}

	rc, err := cas.Open(context.Background(), first.BlobKey)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected hello, got %q", string(data))
	}

	if err := cas.Delete(context.Background(), first.BlobKey); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := cas.Delete(context.Background(), first.BlobKey); err != nil {
		t.Fatalf("delete missing should be noop: %v", err)
	}
}
