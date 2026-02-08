package store

import (
	"context"
	"time"

	"grns/internal/models"
)

// CloseTaskGitRefInput describes one annotation to create while closing tasks atomically.
type CloseTaskGitRefInput struct {
	TaskID         string
	RepoSlug       string
	Relation       string
	ObjectType     string
	ObjectValue    string
	ResolvedCommit string
	Note           string
	Meta           map[string]any
}

// GitRefStore is the persistence surface for task git references.
type GitRefStore interface {
	UpsertGitRepo(ctx context.Context, repo *models.GitRepo) (*models.GitRepo, error)
	GetGitRepoBySlug(ctx context.Context, slug string) (*models.GitRepo, error)

	CreateTaskGitRef(ctx context.Context, ref *models.TaskGitRef) error
	GetTaskGitRef(ctx context.Context, id string) (*models.TaskGitRef, error)
	ListTaskGitRefs(ctx context.Context, taskID string) ([]models.TaskGitRef, error)
	DeleteTaskGitRef(ctx context.Context, id string) error

	CloseTasksWithGitRefs(ctx context.Context, ids []string, closedAt time.Time, refs []CloseTaskGitRefInput) (int, error)
}

var _ GitRefStore = (*Store)(nil)
