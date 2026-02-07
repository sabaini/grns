package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grns/internal/api"
	"grns/internal/models"
	"grns/internal/store"
)

func TestTaskServiceImportSemantics(t *testing.T) {
	t.Run("dry run does not write tasks", func(t *testing.T) {
		svc, st := newTaskServiceForTest(t)
		ctx := context.Background()

		resp, err := svc.Import(ctx, api.ImportRequest{
			DryRun: true,
			Tasks: []api.TaskImportRecord{{
				Task: models.Task{ID: "gr-dr11", Title: "Dry run", Status: "open", Type: "task", Priority: 2},
			}},
		})
		if err != nil {
			t.Fatalf("import dry-run: %v", err)
		}
		if resp.Created != 1 {
			t.Fatalf("expected created=1, got %d", resp.Created)
		}

		exists, err := st.TaskExists("gr-dr11")
		if err != nil {
			t.Fatalf("task exists check: %v", err)
		}
		if exists {
			t.Fatal("expected dry-run to avoid writes")
		}
	})

	t.Run("dedupe skip and error do not rewrite dependencies", func(t *testing.T) {
		tests := []struct {
			name       string
			dedupeMode string
			wantSkip   int
			wantErrors int
		}{
			{name: "skip", dedupeMode: "skip", wantSkip: 1, wantErrors: 0},
			{name: "error", dedupeMode: "error", wantSkip: 0, wantErrors: 1},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				svc, st := newTaskServiceForTest(t)
				ctx := context.Background()
				now := time.Now().UTC()

				mustCreateTask(t, st, &models.Task{ID: "gr-pa11", Title: "Parent one", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}, nil, nil)
				mustCreateTask(t, st, &models.Task{ID: "gr-pa22", Title: "Parent two", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}, nil, nil)
				mustCreateTask(t, st, &models.Task{ID: "gr-ch11", Title: "Child", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}, nil, []models.Dependency{{ParentID: "gr-pa11", Type: "blocks"}})

				resp, err := svc.Import(ctx, api.ImportRequest{
					Dedupe: tt.dedupeMode,
					Tasks: []api.TaskImportRecord{{
						Task: models.Task{ID: "gr-ch11", Title: "Child", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
						Deps: []models.Dependency{{ParentID: "gr-pa22", Type: "blocks"}},
					}},
				})
				if err != nil {
					t.Fatalf("import %s: %v", tt.dedupeMode, err)
				}
				if resp.Skipped != tt.wantSkip {
					t.Fatalf("expected skipped=%d, got %d", tt.wantSkip, resp.Skipped)
				}
				if resp.Errors != tt.wantErrors {
					t.Fatalf("expected errors=%d, got %d", tt.wantErrors, resp.Errors)
				}

				deps, err := st.ListDependencies(ctx, "gr-ch11")
				if err != nil {
					t.Fatalf("list deps: %v", err)
				}
				if len(deps) != 1 || deps[0].ParentID != "gr-pa11" {
					t.Fatalf("dependency should remain on gr-pa11, got %+v", deps)
				}
			})
		}
	})

	t.Run("overwrite deps semantics", func(t *testing.T) {
		t.Run("explicit empty deps clears", func(t *testing.T) {
			svc, st := newTaskServiceForTest(t)
			ctx := context.Background()
			now := time.Now().UTC()

			mustCreateTask(t, st, &models.Task{ID: "gr-pa11", Title: "Parent", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}, nil, nil)
			mustCreateTask(t, st, &models.Task{ID: "gr-ch11", Title: "Child", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}, nil, []models.Dependency{{ParentID: "gr-pa11", Type: "blocks"}})

			_, err := svc.Import(ctx, api.ImportRequest{
				Dedupe: "overwrite",
				Tasks: []api.TaskImportRecord{{
					Task: models.Task{ID: "gr-ch11", Title: "Child", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
					Deps: []models.Dependency{},
				}},
			})
			if err != nil {
				t.Fatalf("import overwrite clear deps: %v", err)
			}

			deps, err := st.ListDependencies(ctx, "gr-ch11")
			if err != nil {
				t.Fatalf("list deps: %v", err)
			}
			if len(deps) != 0 {
				t.Fatalf("expected deps to be cleared, got %+v", deps)
			}
		})

		t.Run("missing deps preserves", func(t *testing.T) {
			svc, st := newTaskServiceForTest(t)
			ctx := context.Background()
			now := time.Now().UTC()

			mustCreateTask(t, st, &models.Task{ID: "gr-pa11", Title: "Parent", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}, nil, nil)
			mustCreateTask(t, st, &models.Task{ID: "gr-ch11", Title: "Child", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}, nil, []models.Dependency{{ParentID: "gr-pa11", Type: "blocks"}})

			_, err := svc.Import(ctx, api.ImportRequest{
				Dedupe: "overwrite",
				Tasks: []api.TaskImportRecord{{
					Task: models.Task{ID: "gr-ch11", Title: "Child renamed", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
				}},
			})
			if err != nil {
				t.Fatalf("import overwrite preserve deps: %v", err)
			}

			deps, err := st.ListDependencies(ctx, "gr-ch11")
			if err != nil {
				t.Fatalf("list deps: %v", err)
			}
			if len(deps) != 1 || deps[0].ParentID != "gr-pa11" {
				t.Fatalf("expected deps to be preserved, got %+v", deps)
			}
		})
	})

	t.Run("status and closed_at are normalized on overwrite", func(t *testing.T) {
		svc, st := newTaskServiceForTest(t)
		ctx := context.Background()
		now := time.Now().UTC()

		mustCreateTask(t, st, &models.Task{ID: "gr-aa11", Title: "Task", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}, nil, nil)

		_, err := svc.Import(ctx, api.ImportRequest{
			Dedupe: "overwrite",
			Tasks: []api.TaskImportRecord{{
				Task: models.Task{ID: "gr-aa11", Title: "Task", Status: "closed", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
			}},
		})
		if err != nil {
			t.Fatalf("import closed overwrite: %v", err)
		}
		task, err := st.GetTask(ctx, "gr-aa11")
		if err != nil {
			t.Fatalf("get task closed: %v", err)
		}
		if task.Status != "closed" {
			t.Fatalf("expected closed status, got %q", task.Status)
		}
		if task.ClosedAt == nil || task.ClosedAt.IsZero() {
			t.Fatal("expected closed_at to be set")
		}

		closedAt := now.Add(-24 * time.Hour)
		_, err = svc.Import(ctx, api.ImportRequest{
			Dedupe: "overwrite",
			Tasks: []api.TaskImportRecord{{
				Task: models.Task{ID: "gr-aa11", Title: "Task", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now, ClosedAt: &closedAt},
			}},
		})
		if err != nil {
			t.Fatalf("import reopen overwrite: %v", err)
		}
		task, err = st.GetTask(ctx, "gr-aa11")
		if err != nil {
			t.Fatalf("get task open: %v", err)
		}
		if task.Status != "open" {
			t.Fatalf("expected open status, got %q", task.Status)
		}
		if task.ClosedAt != nil {
			t.Fatalf("expected closed_at cleared, got %v", task.ClosedAt)
		}
	})

	t.Run("atomic mode rolls back on dependency write failure", func(t *testing.T) {
		tests := []struct {
			name        string
			atomic      bool
			shouldExist bool
		}{
			{name: "best_effort", atomic: false, shouldExist: true},
			{name: "atomic", atomic: true, shouldExist: false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				svc, st := newTaskServiceForTest(t)
				ctx := context.Background()
				now := time.Now().UTC()

				_, err := svc.Import(ctx, api.ImportRequest{
					Atomic: tt.atomic,
					Tasks: []api.TaskImportRecord{{
						Task: models.Task{ID: "gr-at11", Title: "Atomic target", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
						Deps: []models.Dependency{{ParentID: "gr-miss", Type: "blocks"}},
					}},
				})
				if err == nil {
					t.Fatal("expected dependency write failure")
				}

				exists, existsErr := st.TaskExists("gr-at11")
				if existsErr != nil {
					t.Fatalf("task exists check: %v", existsErr)
				}
				if exists != tt.shouldExist {
					t.Fatalf("expected task existence=%v, got %v", tt.shouldExist, exists)
				}
			})
		}
	})

	t.Run("orphan handling strict reports structured error counts and preserves deps", func(t *testing.T) {
		svc, st := newTaskServiceForTest(t)
		ctx := context.Background()
		now := time.Now().UTC()

		resp, err := svc.Import(ctx, api.ImportRequest{
			OrphanHandling: "strict",
			Tasks: []api.TaskImportRecord{{
				Task: models.Task{ID: "gr-or11", Title: "Orphan", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
				Deps: []models.Dependency{{ParentID: "gr-xx99", Type: "blocks"}},
			}},
		})
		if err != nil {
			t.Fatalf("strict orphan import: %v", err)
		}
		if resp.Created != 1 {
			t.Fatalf("expected created=1, got %d", resp.Created)
		}
		if resp.Errors != 1 {
			t.Fatalf("expected errors=1, got %d", resp.Errors)
		}
		if len(resp.Messages) == 0 || !strings.Contains(resp.Messages[0], "strict orphan dep") {
			t.Fatalf("expected strict orphan message, got %#v", resp.Messages)
		}

		task, err := st.GetTask(ctx, "gr-or11")
		if err != nil {
			t.Fatalf("get orphan task: %v", err)
		}
		if task == nil {
			t.Fatal("expected task to exist after strict orphan import")
		}
		deps, err := st.ListDependencies(ctx, "gr-or11")
		if err != nil {
			t.Fatalf("list orphan deps: %v", err)
		}
		if len(deps) != 0 {
			t.Fatalf("expected no deps applied under strict orphan, got %+v", deps)
		}
	})
}

func newTaskServiceForTest(t *testing.T) (*TaskService, *store.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "task_service_test.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	return NewTaskService(st, "gr"), st
}

func mustCreateTask(t *testing.T, st *store.Store, task *models.Task, labels []string, deps []models.Dependency) {
	t.Helper()
	if err := st.CreateTask(context.Background(), task, labels, deps); err != nil {
		t.Fatalf("create task %s: %v", task.ID, err)
	}
}
