package models

import (
	"fmt"
	"strings"
	"time"
)

// GitObjectType describes the referenced git object kind.
type GitObjectType string

const (
	GitObjectTypeCommit GitObjectType = "commit"
	GitObjectTypeTag    GitObjectType = "tag"
	GitObjectTypeBranch GitObjectType = "branch"
	GitObjectTypePath   GitObjectType = "path"
	GitObjectTypeBlob   GitObjectType = "blob"
	GitObjectTypeTree   GitObjectType = "tree"
)

var validGitObjectTypes = map[GitObjectType]struct{}{
	GitObjectTypeCommit: {},
	GitObjectTypeTag:    {},
	GitObjectTypeBranch: {},
	GitObjectTypePath:   {},
	GitObjectTypeBlob:   {},
	GitObjectTypeTree:   {},
}

// GitRepo records canonical git repository identity.
type GitRepo struct {
	ID            string    `json:"id"`
	Slug          string    `json:"slug"`
	DefaultBranch string    `json:"default_branch,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TaskGitRef is a typed link from one task to one git object.
type TaskGitRef struct {
	ID             string         `json:"id"`
	TaskID         string         `json:"task_id"`
	RepoID         string         `json:"repo_id"`
	Repo           string         `json:"repo"`
	Relation       string         `json:"relation"`
	ObjectType     string         `json:"object_type"`
	ObjectValue    string         `json:"object_value"`
	ResolvedCommit string         `json:"resolved_commit,omitempty"`
	Note           string         `json:"note,omitempty"`
	Meta           map[string]any `json:"meta,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// ParseGitObjectType validates and normalizes object types.
func ParseGitObjectType(raw string) (GitObjectType, error) {
	value := GitObjectType(strings.ToLower(strings.TrimSpace(raw)))
	if value == "" {
		return "", fmt.Errorf("object_type is required")
	}
	if _, ok := validGitObjectTypes[value]; !ok {
		return "", fmt.Errorf("invalid object_type: %s", value)
	}
	return value, nil
}
