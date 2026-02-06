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
		{"with offset only", ListFilter{Offset: 1}, 2},
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

func TestDependencyTree(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Create chain: A depends on B, B depends on C.
	for _, task := range []*models.Task{
		{ID: "gr-dt01", Title: "Task A", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "gr-dt02", Title: "Task B", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "gr-dt03", Title: "Task C", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
	} {
		if err := st.CreateTask(ctx, task, nil, nil); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}

	// A depends on B (B blocks A).
	if err := st.AddDependency(ctx, "gr-dt01", "gr-dt02", "blocks"); err != nil {
		t.Fatalf("add dep A->B: %v", err)
	}
	// B depends on C (C blocks B).
	if err := st.AddDependency(ctx, "gr-dt02", "gr-dt03", "blocks"); err != nil {
		t.Fatalf("add dep B->C: %v", err)
	}

	t.Run("from middle node", func(t *testing.T) {
		nodes, err := st.DependencyTree(ctx, "gr-dt02")
		if err != nil {
			t.Fatalf("tree: %v", err)
		}

		upstream := 0
		downstream := 0
		for _, node := range nodes {
			if node.Direction == "upstream" {
				upstream++
			} else {
				downstream++
			}
		}
		if upstream != 1 {
			t.Fatalf("expected 1 upstream, got %d", upstream)
		}
		if downstream != 1 {
			t.Fatalf("expected 1 downstream, got %d", downstream)
		}
	})

	t.Run("from leaf node", func(t *testing.T) {
		nodes, err := st.DependencyTree(ctx, "gr-dt03")
		if err != nil {
			t.Fatalf("tree: %v", err)
		}

		// C has no upstream, but A and B depend on it downstream.
		downstream := 0
		for _, node := range nodes {
			if node.Direction == "downstream" {
				downstream++
			}
		}
		if downstream != 2 {
			t.Fatalf("expected 2 downstream, got %d", downstream)
		}
	})

	t.Run("no deps", func(t *testing.T) {
		noDep := &models.Task{ID: "gr-dt04", Title: "Isolated", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
		if err := st.CreateTask(ctx, noDep, nil, nil); err != nil {
			t.Fatalf("create: %v", err)
		}

		nodes, err := st.DependencyTree(ctx, "gr-dt04")
		if err != nil {
			t.Fatalf("tree: %v", err)
		}
		if len(nodes) != 0 {
			t.Fatalf("expected 0 nodes, got %d", len(nodes))
		}
	})
}

func TestStoreInfo(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Empty store.
	info, err := st.StoreInfo(ctx)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.SchemaVersion == 0 {
		t.Fatal("expected non-zero schema version")
	}
	if info.TotalTasks != 0 {
		t.Fatalf("expected 0 total tasks, got %d", info.TotalTasks)
	}

	// Create some tasks.
	for _, task := range []*models.Task{
		{ID: "gr-in01", Title: "Open 1", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "gr-in02", Title: "Open 2", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "gr-in03", Title: "Closed", Status: "closed", Type: "bug", Priority: 1, CreatedAt: now, UpdatedAt: now},
	} {
		if err := st.CreateTask(ctx, task, nil, nil); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}

	info, err = st.StoreInfo(ctx)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.TotalTasks != 3 {
		t.Fatalf("expected 3 total tasks, got %d", info.TotalTasks)
	}
	if info.TaskCounts["open"] != 2 {
		t.Fatalf("expected 2 open, got %d", info.TaskCounts["open"])
	}
	if info.TaskCounts["closed"] != 1 {
		t.Fatalf("expected 1 closed, got %d", info.TaskCounts["closed"])
	}
}

func TestCleanupClosedTasks(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	old := now.Add(-60 * 24 * time.Hour)

	// Create tasks: 2 old closed, 1 recent closed, 1 open.
	tasks := []*models.Task{
		{ID: "gr-cl01", Title: "Old closed 1", Status: "open", Type: "task", Priority: 2, CreatedAt: old, UpdatedAt: old},
		{ID: "gr-cl02", Title: "Old closed 2", Status: "open", Type: "bug", Priority: 1, CreatedAt: old, UpdatedAt: old},
		{ID: "gr-cl03", Title: "Recent closed", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "gr-cl04", Title: "Still open", Status: "open", Type: "task", Priority: 2, CreatedAt: old, UpdatedAt: old},
	}
	for _, task := range tasks {
		if err := st.CreateTask(ctx, task, nil, nil); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}

	// Add labels to first task to verify cascade.
	if err := st.AddLabels(ctx, "gr-cl01", []string{"important"}); err != nil {
		t.Fatalf("add labels: %v", err)
	}

	// Close three of them.
	if err := st.CloseTasks(ctx, []string{"gr-cl01", "gr-cl02"}, old); err != nil {
		t.Fatalf("close old: %v", err)
	}
	if err := st.CloseTasks(ctx, []string{"gr-cl03"}, now); err != nil {
		t.Fatalf("close recent: %v", err)
	}

	cutoff := now.Add(-30 * 24 * time.Hour)

	t.Run("dry run", func(t *testing.T) {
		result, err := st.CleanupClosedTasks(ctx, cutoff, true)
		if err != nil {
			t.Fatalf("cleanup dry run: %v", err)
		}
		if result.Count != 2 {
			t.Fatalf("expected 2 candidates, got %d", result.Count)
		}
		if !result.DryRun {
			t.Fatal("expected dry_run to be true")
		}

		// Verify nothing was actually deleted.
		all, err := st.ListTasks(ctx, ListFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(all) != 4 {
			t.Fatalf("expected 4 tasks still, got %d", len(all))
		}
	})

	t.Run("actual cleanup", func(t *testing.T) {
		result, err := st.CleanupClosedTasks(ctx, cutoff, false)
		if err != nil {
			t.Fatalf("cleanup: %v", err)
		}
		if result.Count != 2 {
			t.Fatalf("expected 2 deleted, got %d", result.Count)
		}
		if result.DryRun {
			t.Fatal("expected dry_run to be false")
		}

		// Verify only 2 tasks remain.
		all, err := st.ListTasks(ctx, ListFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("expected 2 tasks remaining, got %d", len(all))
		}

		// Verify the right tasks remain.
		for _, task := range all {
			if task.ID == "gr-cl01" || task.ID == "gr-cl02" {
				t.Fatalf("task %s should have been deleted", task.ID)
			}
		}

		// Verify labels were cascade-deleted.
		labels, err := st.ListLabels(ctx, "gr-cl01")
		if err != nil {
			t.Fatalf("list labels: %v", err)
		}
		if len(labels) != 0 {
			t.Fatalf("expected 0 labels after cascade delete, got %d", len(labels))
		}
	})
}

func TestListTasksWithSearch(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	tasks := []*models.Task{
		{ID: "gr-sr01", Title: "Fix authentication bug", Status: "open", Type: "bug", Priority: 0, Description: "Login fails on mobile", CreatedAt: now, UpdatedAt: now},
		{ID: "gr-sr02", Title: "Add caching layer", Status: "open", Type: "feature", Priority: 2, Description: "Redis integration needed", Notes: "authentication tokens should be cached", CreatedAt: now, UpdatedAt: now},
		{ID: "gr-sr03", Title: "Update docs", Status: "open", Type: "task", Priority: 3, Description: "Improve README", CreatedAt: now, UpdatedAt: now},
	}

	for _, task := range tasks {
		if err := st.CreateTask(ctx, task, nil, nil); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}

	t.Run("search title", func(t *testing.T) {
		result, err := st.ListTasks(ctx, ListFilter{SearchQuery: "caching"})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(result) != 1 || result[0].ID != "gr-sr02" {
			t.Fatalf("expected gr-sr02, got %v", result)
		}
	})

	t.Run("search description", func(t *testing.T) {
		result, err := st.ListTasks(ctx, ListFilter{SearchQuery: "mobile"})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(result) != 1 || result[0].ID != "gr-sr01" {
			t.Fatalf("expected gr-sr01, got %v", result)
		}
	})

	t.Run("search notes", func(t *testing.T) {
		result, err := st.ListTasks(ctx, ListFilter{SearchQuery: "tokens"})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(result) != 1 || result[0].ID != "gr-sr02" {
			t.Fatalf("expected gr-sr02, got %v", result)
		}
	})

	t.Run("search across fields", func(t *testing.T) {
		result, err := st.ListTasks(ctx, ListFilter{SearchQuery: "authentication"})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("expected 2 results, got %d", len(result))
		}
	})

	t.Run("search with status filter", func(t *testing.T) {
		result, err := st.ListTasks(ctx, ListFilter{SearchQuery: "authentication", Statuses: []string{"open"}})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("expected 2 results, got %d", len(result))
		}
	})

	t.Run("no results", func(t *testing.T) {
		result, err := st.ListTasks(ctx, ListFilter{SearchQuery: "nonexistent"})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("expected 0 results, got %d", len(result))
		}
	})
}

func TestListTasksWithSpecRegexLimitOffset(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	tasks := []*models.Task{
		{ID: "gr-rx01", Title: "Spec A", Status: "open", Type: "task", Priority: 2, SpecID: "docs/specs/a.md", CreatedAt: now.Add(-3 * time.Minute), UpdatedAt: now.Add(-3 * time.Minute)},
		{ID: "gr-rx02", Title: "Spec B", Status: "open", Type: "task", Priority: 2, SpecID: "docs/specs/b.md", CreatedAt: now.Add(-2 * time.Minute), UpdatedAt: now.Add(-2 * time.Minute)},
		{ID: "gr-rx03", Title: "Other", Status: "open", Type: "task", Priority: 2, SpecID: "notes/c.md", CreatedAt: now.Add(-1 * time.Minute), UpdatedAt: now.Add(-1 * time.Minute)},
	}

	for _, task := range tasks {
		if err := st.CreateTask(ctx, task, nil, nil); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}

	result, err := st.ListTasks(ctx, ListFilter{SpecRegex: "(?i)docs/specs/.*", Offset: 1, Limit: 1})
	if err != nil {
		t.Fatalf("list with spec regex: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].ID != "gr-rx01" {
		t.Fatalf("expected gr-rx01 after offset, got %s", result[0].ID)
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

	// Set custom fields.
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

	// Clear custom fields with empty map.
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

func timePtr(t time.Time) *time.Time { return &t }

func TestListDependenciesForTasks(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	for _, id := range []string{"gr-dp01", "gr-dp02", "gr-dp03"} {
		task := &models.Task{ID: id, Title: id, Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
		if err := st.CreateTask(ctx, task, nil, nil); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	// gr-dp02 depends on gr-dp01; gr-dp03 depends on gr-dp01.
	if err := st.AddDependency(ctx, "gr-dp02", "gr-dp01", "blocks"); err != nil {
		t.Fatalf("add dep: %v", err)
	}
	if err := st.AddDependency(ctx, "gr-dp03", "gr-dp01", "blocks"); err != nil {
		t.Fatalf("add dep: %v", err)
	}

	deps, err := st.ListDependenciesForTasks(ctx, []string{"gr-dp02", "gr-dp03"})
	if err != nil {
		t.Fatalf("list deps: %v", err)
	}
	if len(deps["gr-dp02"]) != 1 || deps["gr-dp02"][0].ParentID != "gr-dp01" {
		t.Fatalf("unexpected deps for gr-dp02: %v", deps["gr-dp02"])
	}
	if len(deps["gr-dp03"]) != 1 || deps["gr-dp03"][0].ParentID != "gr-dp01" {
		t.Fatalf("unexpected deps for gr-dp03: %v", deps["gr-dp03"])
	}
}

func TestReplaceLabels(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	task := &models.Task{ID: "gr-rl01", Title: "Replace labels", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateTask(ctx, task, []string{"alpha", "beta"}, nil); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := st.ReplaceLabels(ctx, "gr-rl01", []string{"gamma", "delta"}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	labels, err := st.ListLabels(ctx, "gr-rl01")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(labels) != 2 || labels[0] != "delta" || labels[1] != "gamma" {
		t.Fatalf("expected [delta, gamma], got %v", labels)
	}
}

func TestRemoveDependencies(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	for _, id := range []string{"gr-rd01", "gr-rd02", "gr-rd03"} {
		task := &models.Task{ID: id, Title: id, Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}
		if err := st.CreateTask(ctx, task, nil, nil); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	if err := st.AddDependency(ctx, "gr-rd01", "gr-rd02", "blocks"); err != nil {
		t.Fatalf("add dep: %v", err)
	}
	if err := st.AddDependency(ctx, "gr-rd01", "gr-rd03", "blocks"); err != nil {
		t.Fatalf("add dep: %v", err)
	}

	if err := st.RemoveDependencies(ctx, "gr-rd01"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	deps, err := st.ListDependencies(ctx, "gr-rd01")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(deps) != 0 {
		t.Fatalf("expected 0 deps, got %d", len(deps))
	}
}

func intPtr(v int) *int { return &v }
