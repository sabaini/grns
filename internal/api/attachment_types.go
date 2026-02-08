package api

import "time"

// AttachmentUploadRequest defines request metadata for multipart managed uploads.
type AttachmentUploadRequest struct {
	Kind      string     `json:"kind"`
	Title     string     `json:"title,omitempty"`
	Filename  string     `json:"filename,omitempty"`
	MediaType string     `json:"media_type,omitempty"`
	Labels    []string   `json:"labels,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// AttachmentCreateLinkRequest defines JSON payload for creating a link/repo attachment.
type AttachmentCreateLinkRequest struct {
	Kind        string         `json:"kind"`
	Title       string         `json:"title,omitempty"`
	Filename    string         `json:"filename,omitempty"`
	MediaType   string         `json:"media_type,omitempty"`
	ExternalURL string         `json:"external_url,omitempty"`
	RepoPath    string         `json:"repo_path,omitempty"`
	Labels      []string       `json:"labels,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
	ExpiresAt   *time.Time     `json:"expires_at,omitempty"`
}

// BlobGCRequest requests one blob garbage-collection run.
type BlobGCRequest struct {
	DryRun    bool `json:"dry_run"`
	BatchSize int  `json:"batch_size,omitempty"`
}

// BlobGCResponse summarizes one blob garbage-collection run.
type BlobGCResponse struct {
	CandidateCount int   `json:"candidate_count"`
	DeletedCount   int   `json:"deleted_count"`
	FailedCount    int   `json:"failed_count"`
	ReclaimedBytes int64 `json:"reclaimed_bytes"`
	DryRun         bool  `json:"dry_run"`
}
