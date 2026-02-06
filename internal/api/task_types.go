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
	Labels             []string            `json:"labels,omitempty"`
	Deps               []models.Dependency `json:"deps,omitempty"`
}

// TaskUpdateRequest defines the payload for updating a task.
type TaskUpdateRequest struct {
	Title              *string `json:"title,omitempty"`
	Status             *string `json:"status,omitempty"`
	Type               *string `json:"type,omitempty"`
	Priority           *int    `json:"priority,omitempty"`
	Description        *string `json:"description,omitempty"`
	SpecID             *string `json:"spec_id,omitempty"`
	ParentID           *string `json:"parent_id,omitempty"`
	Assignee           *string `json:"assignee,omitempty"`
	Notes              *string `json:"notes,omitempty"`
	Design             *string `json:"design,omitempty"`
	AcceptanceCriteria *string `json:"acceptance_criteria,omitempty"`
	SourceRepo         *string `json:"source_repo,omitempty"`
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
