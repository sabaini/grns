package store

import (
	"context"
	"time"

	"grns/internal/models"
)

// StoreInfo holds metadata about the database.
type StoreInfo struct {
	SchemaVersion int            `json:"schema_version"`
	TaskCounts    map[string]int `json:"task_counts"`
	TotalTasks    int            `json:"total_tasks"`
}

// CleanupResult reports on a cleanup operation.
type CleanupResult struct {
	TaskIDs []string `json:"task_ids"`
	Count   int      `json:"count"`
	DryRun  bool     `json:"dry_run"`
}

// TaskStore abstracts task storage backends.
type TaskStore interface {
	StoreInfo(ctx context.Context) (*StoreInfo, error)
	TaskExists(id string) (bool, error)
	CreateTask(ctx context.Context, task *models.Task, labels []string, deps []models.Dependency) error
	GetTask(ctx context.Context, id string) (*models.Task, error)
	UpdateTask(ctx context.Context, id string, update TaskUpdate) error
	ListTasks(ctx context.Context, filter ListFilter) ([]models.Task, error)
	ListReadyTasks(ctx context.Context, limit int) ([]models.Task, error)
	ListStaleTasks(ctx context.Context, cutoff time.Time, statuses []string, limit int) ([]models.Task, error)
	AddLabels(ctx context.Context, id string, labels []string) error
	RemoveLabels(ctx context.Context, id string, labels []string) error
	ListLabels(ctx context.Context, id string) ([]string, error)
	ListAllLabels(ctx context.Context) ([]string, error)
	AddDependency(ctx context.Context, childID, parentID, depType string) error
	ListDependencies(ctx context.Context, id string) ([]models.Dependency, error)
	DependencyTree(ctx context.Context, id string) ([]models.DepTreeNode, error)
	ListLabelsForTasks(ctx context.Context, ids []string) (map[string][]string, error)
	CloseTasks(ctx context.Context, ids []string, closedAt time.Time) error
	ReopenTasks(ctx context.Context, ids []string, reopenedAt time.Time) error
	CleanupClosedTasks(ctx context.Context, cutoff time.Time, dryRun bool) (*CleanupResult, error)
	ListDependenciesForTasks(ctx context.Context, ids []string) (map[string][]models.Dependency, error)
	ReplaceLabels(ctx context.Context, id string, labels []string) error
	RemoveDependencies(ctx context.Context, childID string) error
}

var _ TaskStore = (*Store)(nil)
