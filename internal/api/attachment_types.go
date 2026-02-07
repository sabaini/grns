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
