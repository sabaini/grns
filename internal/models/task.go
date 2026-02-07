package models

import "time"

// Task represents a single task in grns.
type Task struct {
	ID                 string         `json:"id"`
	Title              string         `json:"title"`
	Status             string         `json:"status"`
	Type               string         `json:"type"`
	Priority           int            `json:"priority"`
	Description        string         `json:"description,omitempty"`
	SpecID             string         `json:"spec_id,omitempty"`
	ParentID           string         `json:"parent_id,omitempty"`
	Assignee           string         `json:"assignee,omitempty"`
	Notes              string         `json:"notes,omitempty"`
	Design             string         `json:"design,omitempty"`
	AcceptanceCriteria string         `json:"acceptance_criteria,omitempty"`
	SourceRepo         string         `json:"source_repo,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	ClosedAt           *time.Time     `json:"closed_at,omitempty"`
	Custom             map[string]any `json:"custom,omitempty"`
}
