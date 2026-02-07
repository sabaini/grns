package store

import (
	"context"
	"testing"
	"time"

	"grns/internal/models"
)

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

	if len(ready) != 2 {
		t.Fatalf("expected 2 ready tasks, got %d", len(ready))
	}
	for _, task := range ready {
		if task.ID == "gr-bk00" {
			t.Fatal("blocked task should not be ready")
		}
	}
}

func TestListTasksExtendedFilters(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	earlier := now.Add(-48 * time.Hour)

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
