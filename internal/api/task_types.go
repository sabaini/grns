package api

import "grns/internal/models"

// TaskCreateRequest defines the payload for creating a task.
type TaskCreateRequest struct {
	ID                 string              `json:"id,omitempty"`
	Title              string              `json:"title"`
	Status             *string             `json:"status,omitempty"`
	Type               *string             `json:"type,omitempty"`
	Priority           *int                `json:"priority,omitempty"`
	Description        *string             `json:"description,omitempty"`
	SpecID             *string             `json:"spec_id,omitempty"`
	ParentID           *string             `json:"parent_id,omitempty"`
	Assignee           *string             `json:"assignee,omitempty"`
	Notes              *string             `json:"notes,omitempty"`
	Design             *string             `json:"design,omitempty"`
	AcceptanceCriteria *string             `json:"acceptance_criteria,omitempty"`
	SourceRepo         *string             `json:"source_repo,omitempty"`
	Custom             map[string]any      `json:"custom,omitempty"`
	Labels             []string            `json:"labels,omitempty"`
	Deps               []models.Dependency `json:"deps,omitempty"`
}

// TaskUpdateRequest defines the payload for updating a task.
type TaskUpdateRequest struct {
	Title              *string        `json:"title,omitempty"`
	Status             *string        `json:"status,omitempty"`
	Type               *string        `json:"type,omitempty"`
	Priority           *int           `json:"priority,omitempty"`
	Description        *string        `json:"description,omitempty"`
	SpecID             *string        `json:"spec_id,omitempty"`
	ParentID           *string        `json:"parent_id,omitempty"`
	Assignee           *string        `json:"assignee,omitempty"`
	Notes              *string        `json:"notes,omitempty"`
	Design             *string        `json:"design,omitempty"`
	AcceptanceCriteria *string        `json:"acceptance_criteria,omitempty"`
	SourceRepo         *string        `json:"source_repo,omitempty"`
	Custom             map[string]any `json:"custom,omitempty"`
}

// InfoResponse is the response from GET /v1/info.
type InfoResponse struct {
	ProjectPrefix string         `json:"project_prefix"`
	SchemaVersion int            `json:"schema_version"`
	TaskCounts    map[string]int `json:"task_counts"`
	TotalTasks    int            `json:"total_tasks"`
}

// DepTreeResponse wraps the dependency tree output.
type DepTreeResponse struct {
	RootID string               `json:"root_id"`
	Nodes  []models.DepTreeNode `json:"nodes"`
}

// CleanupRequest defines the payload for admin cleanup.
type CleanupRequest struct {
	OlderThanDays int  `json:"older_than_days"`
	DryRun        bool `json:"dry_run"`
}

// CleanupResponse is the response from POST /v1/admin/cleanup.
type CleanupResponse struct {
	TaskIDs []string `json:"task_ids"`
	Count   int      `json:"count"`
	DryRun  bool     `json:"dry_run"`
}

// TaskCloseRequest defines the payload for closing tasks.
type TaskCloseRequest struct {
	IDs []string `json:"ids"`
}

// TaskReopenRequest defines the payload for reopening tasks.
type TaskReopenRequest struct {
	IDs []string `json:"ids"`
}

// LabelsRequest defines label add/remove payloads.
type LabelsRequest struct {
	Labels []string `json:"labels"`
}

// DepCreateRequest defines dependency creation payload.
type DepCreateRequest struct {
	ChildID  string `json:"child_id"`
	ParentID string `json:"parent_id"`
	Type     string `json:"type,omitempty"`
}

// TaskResponse wraps a task with labels and dependencies.
type TaskResponse struct {
	models.Task
	Labels []string            `json:"labels"`
	Deps   []models.Dependency `json:"deps,omitempty"`
}

// TaskGetManyRequest defines payload for bulk task retrieval.
type TaskGetManyRequest struct {
	IDs []string `json:"ids"`
}

// TaskImportRecord represents one task in an import payload.
type TaskImportRecord struct {
	models.Task
	Labels []string            `json:"labels"`
	Deps   []models.Dependency `json:"deps"`
}

// ImportRequest is the payload for POST /v1/import.
type ImportRequest struct {
	Tasks          []TaskImportRecord `json:"tasks"`
	DryRun         bool               `json:"dry_run"`
	Dedupe         string             `json:"dedupe"`
	OrphanHandling string             `json:"orphan_handling"`
	Atomic         bool               `json:"atomic,omitempty"`
}

// ImportResponse is the response from POST /v1/import.
type ImportResponse struct {
	Created       int      `json:"created"`
	Updated       int      `json:"updated"`
	Skipped       int      `json:"skipped"`
	Errors        int      `json:"errors"`
	DryRun        bool     `json:"dry_run"`
	TaskIDs       []string `json:"task_ids"`
	Messages      []string `json:"messages,omitempty"`
	ApplyMode     string   `json:"apply_mode,omitempty"`
	AppliedChunks int      `json:"applied_chunks,omitempty"`
}
