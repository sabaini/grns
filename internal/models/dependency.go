package models

// Dependency represents a relationship between tasks.
type Dependency struct {
	ParentID string `json:"parent_id"`
	Type     string `json:"type"`
}
