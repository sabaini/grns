package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"grns/internal/models"
)

// testStore creates a temporary store for testing.
func testStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

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

func TestListTasks(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	for _, task := range []*models.Task{
		{ID: "gr-ls01", Title: "Bug A", Status: "open", Type: "bug", Priority: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "gr-ls02", Title: "Task B", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "gr-ls03", Title: "Bug C", Status: "closed", Type: "bug", Priority: 1, CreatedAt: now, UpdatedAt: now},
	} {
		if err := st.CreateTask(ctx, task, nil, nil); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}

	tests := []struct {
		name   string
		filter ListFilter
		want   int
	}{
		{"all", ListFilter{}, 3},
		{"by status", ListFilter{Statuses: []string{"open"}}, 2},
		{"by type", ListFilter{Types: []string{"bug"}}, 2},
		{"by priority", ListFilter{Priority: intPtr(0)}, 1},
		{"with limit", ListFilter{Limit: 1}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks, err := st.ListTasks(ctx, tt.filter)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(tasks) != tt.want {
				t.Fatalf("expected %d tasks, got %d", tt.want, len(tasks))
			}
		})
	}
}

func TestReadyTasks(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	blocker := &models.Task{ID: "gr-bl00", Title: "Blocker", Status: "open", Type: "task", Priority: 1, CreatedAt: now, UpdatedAt: now}
	blocked := &models.Task{ID: "gr-bk00", Title: "Blocked", Status: "open", Type: "task", Priority: 1, CreatedAt: now, UpdatedAt: now}
	free := &models.Task{ID: "gr-fr00", Title: "Free", Status: "open", Type: "task", Priority: 1, CreatedAt: now, UpdatedAt: now}

	for _, task := range []*models.Task{blocker, blocked, free} {
		if err := st.CreateTask(ctx, task, nil, nil); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}

	if err := st.AddDependency(ctx, "gr-bk00", "gr-bl00", "blocks"); err != nil {
		t.Fatalf("add dep: %v", err)
	}

	ready, err := st.ListReadyTasks(ctx, 0)
	if err != nil {
		t.Fatalf("ready: %v", err)
	}

	// blocker and free should be ready; blocked should not
	if len(ready) != 2 {
		t.Fatalf("expected 2 ready tasks, got %d", len(ready))
	}
	for _, task := range ready {
		if task.ID == "gr-bk00" {
			t.Fatal("blocked task should not be ready")
		}
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

	// Clear a field by setting to empty string.
	empty := ""
	if err := st.UpdateTask(ctx, "gr-uf01", TaskUpdate{Assignee: &empty, UpdatedAt: now}); err != nil {
		t.Fatalf("clear assignee: %v", err)
	}

	got, _ = st.GetTask(ctx, "gr-uf01")
	if got.Assignee != "" {
		t.Fatalf("expected empty assignee, got %q", got.Assignee)
	}
}

func TestListTasksExtendedFilters(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	earlier := now.Add(-48 * time.Hour)

	// Create test tasks with various fields.
	tasks := []*models.Task{
		{ID: "gr-ef01", Title: "Bug report", Status: "open", Type: "bug", Priority: 0, Assignee: "alice", Description: "a bug", CreatedAt: earlier, UpdatedAt: earlier},
		{ID: "gr-ef02", Title: "Feature request", Status: "open", Type: "feature", Priority: 2, Assignee: "bob", Notes: "important notes", CreatedAt: now, UpdatedAt: now},
		{ID: "gr-ef03", Title: "Unassigned task", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "gr-ef04", Title: "No description task", Status: "open", Type: "task", Priority: 1, Description: "", CreatedAt: now, UpdatedAt: now},
	}

	for _, task := range tasks {
		if err := st.CreateTask(ctx, task, nil, nil); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}

	// Add labels to first two tasks only.
	if err := st.AddLabels(ctx, "gr-ef01", []string{"critical"}); err != nil {
		t.Fatalf("add labels: %v", err)
	}
	if err := st.AddLabels(ctx, "gr-ef02", []string{"enhancement"}); err != nil {
		t.Fatalf("add labels: %v", err)
	}

	tests := []struct {
		name   string
		filter ListFilter
		want   int
	}{
		{"by assignee", ListFilter{Assignee: "alice"}, 1},
		{"no assignee", ListFilter{NoAssignee: true}, 2},
		{"by ids", ListFilter{IDs: []string{"gr-ef01", "gr-ef03"}}, 2},
		{"title contains", ListFilter{TitleContains: "Bug"}, 1},
		{"desc contains", ListFilter{DescContains: "bug"}, 1},
		{"notes contains", ListFilter{NotesContains: "important"}, 1},
		{"created after", ListFilter{CreatedAfter: timePtr(earlier.Add(time.Hour))}, 3},
		{"created before", ListFilter{CreatedBefore: timePtr(earlier.Add(time.Hour))}, 1},
		{"empty description", ListFilter{EmptyDescription: true}, 3},
		{"no labels", ListFilter{NoLabels: true}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := st.ListTasks(ctx, tt.filter)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(result) != tt.want {
				ids := make([]string, len(result))
				for i, task := range result {
					ids[i] = task.ID
				}
				t.Fatalf("expected %d tasks, got %d: %v", tt.want, len(result), ids)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time { return &t }

func intPtr(v int) *int { return &v }
