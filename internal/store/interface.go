package store

import (
	"context"
	"time"

	"grns/internal/models"
)

// TaskStore abstracts task storage backends.
type TaskStore interface {
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
	ListLabelsForTasks(ctx context.Context, ids []string) (map[string][]string, error)
	CloseTasks(ctx context.Context, ids []string, closedAt time.Time) error
	ReopenTasks(ctx context.Context, ids []string, reopenedAt time.Time) error
}

var _ TaskStore = (*Store)(nil)
