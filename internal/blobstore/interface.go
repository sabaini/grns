package blobstore

import (
	"context"
	"io"
)

// BlobPutResult describes one persisted blob payload.
type BlobPutResult struct {
	SHA256    string
	SizeBytes int64
	BlobKey   string
}

// BlobStore is the byte-storage abstraction used by AttachmentService.
type BlobStore interface {
	Put(ctx context.Context, r io.Reader) (BlobPutResult, error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}
