package models

import "time"

// Task represents a single issue/task in grns.
type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Type        string    `json:"type"`
	Priority    int       `json:"priority"`
	Description string    `json:"description,omitempty"`
	SpecID      string    `json:"spec_id,omitempty"`
	ParentID    string    `json:"parent_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
}
