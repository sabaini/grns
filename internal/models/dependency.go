package models

// Dependency represents a relationship between tasks.
type Dependency struct {
	ParentID string `json:"parent_id"`
	Type     string `json:"type"`
}

// DepTreeNode represents a single node in a dependency tree walk.
type DepTreeNode struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Type      string `json:"type"`
	Depth     int    `json:"depth"`
	Direction string `json:"direction"`
	DepType   string `json:"dep_type"`
}
