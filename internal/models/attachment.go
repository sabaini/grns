package models

import (
	"fmt"
	"strings"
	"time"
)

// AttachmentKind describes the domain intent of an attachment.
type AttachmentKind string

const (
	AttachmentKindSpec       AttachmentKind = "spec"
	AttachmentKindDiagram    AttachmentKind = "diagram"
	AttachmentKindArtifact   AttachmentKind = "artifact"
	AttachmentKindDiagnostic AttachmentKind = "diagnostic"
	AttachmentKindArchive    AttachmentKind = "archive"
	AttachmentKindOther      AttachmentKind = "other"
)

// AttachmentSourceType describes where an attachment is sourced from.
type AttachmentSourceType string

const (
	AttachmentSourceManagedBlob AttachmentSourceType = "managed_blob"
	AttachmentSourceExternalURL AttachmentSourceType = "external_url"
	AttachmentSourceRepoPath    AttachmentSourceType = "repo_path"
)

// AttachmentMediaTypeSource records how media_type was determined.
type AttachmentMediaTypeSource string

const (
	MediaTypeSourceSniffed  AttachmentMediaTypeSource = "sniffed"
	MediaTypeSourceDeclared AttachmentMediaTypeSource = "declared"
	MediaTypeSourceInferred AttachmentMediaTypeSource = "inferred"
	MediaTypeSourceUnknown  AttachmentMediaTypeSource = "unknown"
)

var validAttachmentKinds = map[AttachmentKind]struct{}{
	AttachmentKindSpec:       {},
	AttachmentKindDiagram:    {},
	AttachmentKindArtifact:   {},
	AttachmentKindDiagnostic: {},
	AttachmentKindArchive:    {},
	AttachmentKindOther:      {},
}

var validAttachmentSourceTypes = map[AttachmentSourceType]struct{}{
	AttachmentSourceManagedBlob: {},
	AttachmentSourceExternalURL: {},
	AttachmentSourceRepoPath:    {},
}

var validAttachmentMediaTypeSources = map[AttachmentMediaTypeSource]struct{}{
	MediaTypeSourceSniffed:  {},
	MediaTypeSourceDeclared: {},
	MediaTypeSourceInferred: {},
	MediaTypeSourceUnknown:  {},
}

// Attachment is the user-facing attachment reference linked to a task.
type Attachment struct {
	Project         string         `json:"project,omitempty"`
	ID              string         `json:"id"`
	TaskID          string         `json:"task_id"`
	Kind            string         `json:"kind"`
	SourceType      string         `json:"source_type"`
	Title           string         `json:"title,omitempty"`
	Filename        string         `json:"filename,omitempty"`
	MediaType       string         `json:"media_type,omitempty"`
	MediaTypeSource string         `json:"media_type_source,omitempty"`
	BlobID          string         `json:"blob_id,omitempty"`
	ExternalURL     string         `json:"external_url,omitempty"`
	RepoPath        string         `json:"repo_path,omitempty"`
	Meta            map[string]any `json:"meta,omitempty"`
	Labels          []string       `json:"labels,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	ExpiresAt       *time.Time     `json:"expires_at,omitempty"`
}

func ParseAttachmentKind(raw string) (AttachmentKind, error) {
	value := AttachmentKind(strings.ToLower(strings.TrimSpace(raw)))
	if value == "" {
		return "", fmt.Errorf("attachment kind is required")
	}
	if _, ok := validAttachmentKinds[value]; !ok {
		return "", fmt.Errorf("invalid attachment kind: %s", value)
	}
	return value, nil
}

func ParseAttachmentSourceType(raw string) (AttachmentSourceType, error) {
	value := AttachmentSourceType(strings.ToLower(strings.TrimSpace(raw)))
	if value == "" {
		return "", fmt.Errorf("attachment source_type is required")
	}
	if _, ok := validAttachmentSourceTypes[value]; !ok {
		return "", fmt.Errorf("invalid attachment source_type: %s", value)
	}
	return value, nil
}

func ParseAttachmentMediaTypeSource(raw string) (AttachmentMediaTypeSource, error) {
	value := AttachmentMediaTypeSource(strings.ToLower(strings.TrimSpace(raw)))
	if value == "" {
		return "", fmt.Errorf("attachment media_type_source is required")
	}
	if _, ok := validAttachmentMediaTypeSources[value]; !ok {
		return "", fmt.Errorf("invalid attachment media_type_source: %s", value)
	}
	return value, nil
}
