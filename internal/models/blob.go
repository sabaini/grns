package models

import "time"

// Blob is an immutable stored content object referenced by attachments.
type Blob struct {
	ID             string    `json:"id"`
	SHA256         string    `json:"sha256"`
	SizeBytes      int64     `json:"size_bytes"`
	StorageBackend string    `json:"storage_backend"`
	BlobKey        string    `json:"blob_key"`
	CreatedAt      time.Time `json:"created_at"`
}
