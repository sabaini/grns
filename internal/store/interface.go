package store

import (
	"context"
	"errors"
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

// ErrTaskNotFound indicates no matching tasks were found for a mutation.
var ErrTaskNotFound = errors.New("task not found")

// TaskCreateInput defines one task create operation with related labels and dependencies.
type TaskCreateInput struct {
	Task   *models.Task
	Labels []string
	Deps   []models.Dependency
}

// ImportMutator is the transactional mutation subset used by import atomic mode.
type ImportMutator interface {
	TaskExists(id string) (bool, error)
	CreateTask(ctx context.Context, task *models.Task, labels []string, deps []models.Dependency) error
	UpdateTask(ctx context.Context, id string, update TaskUpdate) error
	AddDependency(ctx context.Context, childID, parentID, depType string) error
	ReplaceLabels(ctx context.Context, id string, labels []string) error
	RemoveDependencies(ctx context.Context, childID string) error
}

// ImportStore is the narrowed import capability dependency used by the importer.
type ImportStore interface {
	ImportMutator
	RunInTx(ctx context.Context, fn func(ImportMutator) error) error
}

// TaskServiceStore is the narrowed store capability surface required by TaskService.
type TaskServiceStore interface {
	ImportStore
	CreateTasks(ctx context.Context, tasks []TaskCreateInput) error
	GetTask(ctx context.Context, id string) (*models.Task, error)
	ListTasks(ctx context.Context, filter ListFilter) ([]models.Task, error)
	ListReadyTasks(ctx context.Context, limit int) ([]models.Task, error)
	ListStaleTasks(ctx context.Context, cutoff time.Time, statuses []string, limit int) ([]models.Task, error)
	AddLabels(ctx context.Context, id string, labels []string) error
	RemoveLabels(ctx context.Context, id string, labels []string) error
	ListLabels(ctx context.Context, id string) ([]string, error)
	ListDependencies(ctx context.Context, id string) ([]models.Dependency, error)
	ListLabelsForTasks(ctx context.Context, ids []string) (map[string][]string, error)
	ListDependenciesForTasks(ctx context.Context, ids []string) (map[string][]models.Dependency, error)
	CloseTasks(ctx context.Context, ids []string, closedAt time.Time) error
	ReopenTasks(ctx context.Context, ids []string, reopenedAt time.Time) error
}

// TaskStore abstracts task storage backends.
type TaskStore interface {
	TaskServiceStore
	StoreInfo(ctx context.Context) (*StoreInfo, error)
	ListAllLabels(ctx context.Context) ([]string, error)
	DependencyTree(ctx context.Context, id string) ([]models.DepTreeNode, error)
	CleanupClosedTasks(ctx context.Context, cutoff time.Time, dryRun bool) (*CleanupResult, error)
}

var _ ImportStore = (*Store)(nil)
var _ TaskServiceStore = (*Store)(nil)
var _ TaskStore = (*Store)(nil)
