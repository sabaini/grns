package api

import "grns/internal/models"

// TaskGitRefCreateRequest defines payload for creating a task git reference.
type TaskGitRefCreateRequest struct {
	Repo           string         `json:"repo,omitempty"`
	Relation       string         `json:"relation"`
	ObjectType     string         `json:"object_type"`
	ObjectValue    string         `json:"object_value"`
	ResolvedCommit string         `json:"resolved_commit,omitempty"`
	Note           string         `json:"note,omitempty"`
	Meta           map[string]any `json:"meta,omitempty"`
}

// TaskGitRefResponse returns one task git reference.
type TaskGitRefResponse struct {
	models.TaskGitRef
}
