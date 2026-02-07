package store

import (
	"context"
	"testing"
	"time"

	"grns/internal/models"
)

func TestCreateAndGetTask(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	task := &models.Task{
		ID:        "gr-ab12",
		Title:     "Test task",
		Status:    "open",
		Type:      "task",
		Priority:  2,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := st.GetTask(ctx, "gr-ab12")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected task, got nil")
	}
	if got.Title != "Test task" {
		t.Fatalf("expected title 'Test task', got %q", got.Title)
	}
	if got.Status != "open" {
		t.Fatalf("expected status 'open', got %q", got.Status)
	}
}

func TestCreateTaskWithLabelsAndDeps(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	parent := &models.Task{ID: "gr-pp00", Title: "Parent", Status: "open", Type: "task", Priority: 1, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateTask(ctx, parent, nil, nil); err != nil {
		t.Fatalf("create parent: %v", err)
	}

	child := &models.Task{ID: "gr-cc00", Title: "Child", Status: "open", Type: "bug", Priority: 0, CreatedAt: now, UpdatedAt: now}
	labels := []string{"critical", "backend"}
	deps := []models.Dependency{{ParentID: "gr-pp00", Type: "blocks"}}

	if err := st.CreateTask(ctx, child, labels, deps); err != nil {
		t.Fatalf("create child: %v", err)
	}

	gotLabels, err := st.ListLabels(ctx, "gr-cc00")
	if err != nil {
		t.Fatalf("list labels: %v", err)
	}
	if len(gotLabels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(gotLabels))
	}

	gotDeps, err := st.ListDependencies(ctx, "gr-cc00")
	if err != nil {
		t.Fatalf("list deps: %v", err)
	}
	if len(gotDeps) != 1 || gotDeps[0].ParentID != "gr-pp00" {
		t.Fatalf("expected dep on gr-pp00, got %+v", gotDeps)
	}
}

func TestCreateTasksAtomicOnError(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	tasks := []TaskCreateInput{
		{
			Task: &models.Task{ID: "gr-bt01", Title: "First", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
		},
		{
			Task: &models.Task{ID: "gr-bt01", Title: "Duplicate", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
		},
	}

	if err := st.CreateTasks(ctx, tasks); err == nil {
		t.Fatal("expected duplicate-id error")
	}

	got, err := st.ListTasks(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("list after failed create: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected rollback to keep 0 tasks, got %d", len(got))
	}
}

func TestUpdateTask(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	task := &models.Task{ID: "gr-up00", Title: "Original", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create: %v", err)
	}

	newTitle := "Updated"
	newStatus := "in_progress"
	if err := st.UpdateTask(ctx, "gr-up00", TaskUpdate{Title: &newTitle, Status: &newStatus, UpdatedAt: now}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := st.GetTask(ctx, "gr-up00")
	if got.Title != "Updated" {
		t.Fatalf("expected title 'Updated', got %q", got.Title)
	}
	if got.Status != "in_progress" {
		t.Fatalf("expected status 'in_progress', got %q", got.Status)
	}
}

func TestCloseAndReopen(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	task := &models.Task{ID: "gr-cr00", Title: "Closable", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := st.CloseTasks(ctx, []string{"gr-cr00"}, now); err != nil {
		t.Fatalf("close: %v", err)
	}

	got, _ := st.GetTask(ctx, "gr-cr00")
	if got.Status != "closed" {
		t.Fatalf("expected closed, got %q", got.Status)
	}
	if got.ClosedAt == nil {
		t.Fatal("expected closed_at to be set")
	}

	if err := st.ReopenTasks(ctx, []string{"gr-cr00"}, now); err != nil {
		t.Fatalf("reopen: %v", err)
	}

	got, _ = st.GetTask(ctx, "gr-cr00")
	if got.Status != "open" {
		t.Fatalf("expected open, got %q", got.Status)
	}
	if got.ClosedAt != nil {
		t.Fatal("expected closed_at to be nil")
	}
}

func TestCloseAndReopenMissingTask(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	if err := st.CloseTasks(ctx, []string{"gr-zzzz"}, now); err != ErrTaskNotFound {
		t.Fatalf("expected ErrTaskNotFound on close, got %v", err)
	}
	if err := st.ReopenTasks(ctx, []string{"gr-zzzz"}, now); err != ErrTaskNotFound {
		t.Fatalf("expected ErrTaskNotFound on reopen, got %v", err)
	}
}

func TestCloseAndReopenAllOrNothingWithMissingIDs(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	task := &models.Task{ID: "gr-mx11", Title: "Mixed close", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := st.CloseTasks(ctx, []string{"gr-mx11", "gr-mx99"}, now); err != ErrTaskNotFound {
		t.Fatalf("expected ErrTaskNotFound on mixed close, got %v", err)
	}
	got, err := st.GetTask(ctx, "gr-mx11")
	if err != nil {
		t.Fatalf("get after failed close: %v", err)
	}
	if got.Status != "open" {
		t.Fatalf("expected task to remain open after failed mixed close, got %q", got.Status)
	}

	if err := st.CloseTasks(ctx, []string{"gr-mx11"}, now); err != nil {
		t.Fatalf("close existing: %v", err)
	}
	if err := st.ReopenTasks(ctx, []string{"gr-mx11", "gr-mx99"}, now); err != ErrTaskNotFound {
		t.Fatalf("expected ErrTaskNotFound on mixed reopen, got %v", err)
	}
	got, err = st.GetTask(ctx, "gr-mx11")
	if err != nil {
		t.Fatalf("get after failed reopen: %v", err)
	}
	if got.Status != "closed" {
		t.Fatalf("expected task to remain closed after failed mixed reopen, got %q", got.Status)
	}
}

func TestCreateAndGetTaskWithNewFields(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	task := &models.Task{
		ID:                 "gr-nf01",
		Title:              "New fields test",
		Status:             "open",
		Type:               "task",
		Priority:           2,
		Assignee:           "alice",
		Notes:              "some notes",
		Design:             "design doc",
		AcceptanceCriteria: "it works",
		SourceRepo:         "github.com/test/repo",
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := st.GetTask(ctx, "gr-nf01")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected task, got nil")
	}
	if got.Assignee != "alice" {
		t.Fatalf("expected assignee 'alice', got %q", got.Assignee)
	}
	if got.Notes != "some notes" {
		t.Fatalf("expected notes 'some notes', got %q", got.Notes)
	}
	if got.Design != "design doc" {
		t.Fatalf("expected design 'design doc', got %q", got.Design)
	}
	if got.AcceptanceCriteria != "it works" {
		t.Fatalf("expected acceptance_criteria 'it works', got %q", got.AcceptanceCriteria)
	}
	if got.SourceRepo != "github.com/test/repo" {
		t.Fatalf("expected source_repo 'github.com/test/repo', got %q", got.SourceRepo)
	}
}

func TestUpdateTaskNewFields(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	task := &models.Task{ID: "gr-uf01", Title: "Update fields", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create: %v", err)
	}

	assignee := "bob"
	notes := "updated notes"
	design := "new design"
	ac := "acceptance criteria"
	repo := "github.com/updated"
	if err := st.UpdateTask(ctx, "gr-uf01", TaskUpdate{
		Assignee:           &assignee,
		Notes:              &notes,
		Design:             &design,
		AcceptanceCriteria: &ac,
		SourceRepo:         &repo,
		UpdatedAt:          now,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := st.GetTask(ctx, "gr-uf01")
	if got.Assignee != "bob" {
		t.Fatalf("expected assignee 'bob', got %q", got.Assignee)
	}
	if got.Notes != "updated notes" {
		t.Fatalf("expected notes 'updated notes', got %q", got.Notes)
	}
	if got.Design != "new design" {
		t.Fatalf("expected design 'new design', got %q", got.Design)
	}
	if got.AcceptanceCriteria != "acceptance criteria" {
		t.Fatalf("expected ac 'acceptance criteria', got %q", got.AcceptanceCriteria)
	}
	if got.SourceRepo != "github.com/updated" {
		t.Fatalf("expected source_repo 'github.com/updated', got %q", got.SourceRepo)
	}

	empty := ""
	if err := st.UpdateTask(ctx, "gr-uf01", TaskUpdate{Assignee: &empty, UpdatedAt: now}); err != nil {
		t.Fatalf("clear assignee: %v", err)
	}

	got, _ = st.GetTask(ctx, "gr-uf01")
	if got.Assignee != "" {
		t.Fatalf("expected empty assignee, got %q", got.Assignee)
	}
}

func TestCreateTaskWithCustom(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	task := &models.Task{
		ID:        "gr-cu01",
		Title:     "Custom fields test",
		Status:    "open",
		Type:      "task",
		Priority:  2,
		Custom:    map[string]any{"env": "prod", "team": "backend"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := st.GetTask(ctx, "gr-cu01")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Custom == nil {
		t.Fatal("expected non-nil custom map")
	}
	if got.Custom["env"] != "prod" {
		t.Fatalf("expected env=prod, got %v", got.Custom["env"])
	}
	if got.Custom["team"] != "backend" {
		t.Fatalf("expected team=backend, got %v", got.Custom["team"])
	}
}

func TestUpdateTaskCustom(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	task := &models.Task{
		ID:        "gr-cu02",
		Title:     "Custom update test",
		Status:    "open",
		Type:      "task",
		Priority:  2,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create: %v", err)
	}

	custom := map[string]any{"env": "staging", "version": "1.0"}
	if err := st.UpdateTask(ctx, "gr-cu02", TaskUpdate{
		Custom:    &custom,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := st.GetTask(ctx, "gr-cu02")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Custom["env"] != "staging" {
		t.Fatalf("expected env=staging, got %v", got.Custom["env"])
	}

	empty := map[string]any{}
	if err := st.UpdateTask(ctx, "gr-cu02", TaskUpdate{
		Custom:    &empty,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, err = st.GetTask(ctx, "gr-cu02")
	if err != nil {
		t.Fatalf("get after clear: %v", err)
	}
	if len(got.Custom) != 0 {
		t.Fatalf("expected empty custom, got %v", got.Custom)
	}
}
