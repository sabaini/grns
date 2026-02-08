package store

import (
	"context"
	"testing"
	"time"

	"grns/internal/models"
)

func TestStoreInfo(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

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

func TestCleanupClosedTasksProjectFilter(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	old := now.Add(-60 * 24 * time.Hour)

	for _, task := range []*models.Task{
		{ID: "gr-cpf1", Title: "gr old closed", Status: "closed", Type: "task", Priority: 2, CreatedAt: old, UpdatedAt: old, ClosedAt: &old},
		{ID: "xy-cpf1", Title: "xy old closed", Status: "closed", Type: "task", Priority: 2, CreatedAt: old, UpdatedAt: old, ClosedAt: &old},
	} {
		if err := st.CreateTask(ctx, task, nil, nil); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}

	cutoff := now.Add(-30 * 24 * time.Hour)
	result, err := st.CleanupClosedTasks(ctx, "gr", cutoff, false)
	if err != nil {
		t.Fatalf("cleanup project filter: %v", err)
	}
	if result.Count != 1 || len(result.TaskIDs) != 1 || result.TaskIDs[0] != "gr-cpf1" {
		t.Fatalf("expected to delete only gr-cpf1, got %#v", result)
	}

	gr, err := st.GetTask(ctx, "gr-cpf1")
	if err != nil {
		t.Fatalf("get gr task: %v", err)
	}
	if gr != nil {
		t.Fatal("expected gr-cpf1 to be deleted")
	}

	xy, err := st.GetTask(ctx, "xy-cpf1")
	if err != nil {
		t.Fatalf("get xy task: %v", err)
	}
	if xy == nil {
		t.Fatal("expected xy-cpf1 to remain")
	}
}

func TestCleanupClosedTasks(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	old := now.Add(-60 * 24 * time.Hour)

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

	if err := st.AddLabels(ctx, "gr-cl01", []string{"important"}); err != nil {
		t.Fatalf("add labels: %v", err)
	}

	if err := st.CloseTasks(ctx, "gr", []string{"gr-cl01", "gr-cl02"}, old); err != nil {
		t.Fatalf("close old: %v", err)
	}
	if err := st.CloseTasks(ctx, "gr", []string{"gr-cl03"}, now); err != nil {
		t.Fatalf("close recent: %v", err)
	}

	cutoff := now.Add(-30 * 24 * time.Hour)

	t.Run("dry run", func(t *testing.T) {
		result, err := st.CleanupClosedTasks(ctx, "", cutoff, true)
		if err != nil {
			t.Fatalf("cleanup dry run: %v", err)
		}
		if result.Count != 2 {
			t.Fatalf("expected 2 candidates, got %d", result.Count)
		}
		if !result.DryRun {
			t.Fatal("expected dry_run to be true")
		}

		all, err := st.ListTasks(ctx, ListFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(all) != 4 {
			t.Fatalf("expected 4 tasks still, got %d", len(all))
		}
	})

	t.Run("actual cleanup", func(t *testing.T) {
		result, err := st.CleanupClosedTasks(ctx, "", cutoff, false)
		if err != nil {
			t.Fatalf("cleanup: %v", err)
		}
		if result.Count != 2 {
			t.Fatalf("expected 2 deleted, got %d", result.Count)
		}
		if result.DryRun {
			t.Fatal("expected dry_run to be false")
		}

		all, err := st.ListTasks(ctx, ListFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("expected 2 tasks remaining, got %d", len(all))
		}

		for _, task := range all {
			if task.ID == "gr-cl01" || task.ID == "gr-cl02" {
				t.Fatalf("task %s should have been deleted", task.ID)
			}
		}

		labels, err := st.ListLabels(ctx, "gr-cl01")
		if err != nil {
			t.Fatalf("list labels: %v", err)
		}
		if len(labels) != 0 {
			t.Fatalf("expected 0 labels after cascade delete, got %d", len(labels))
		}
	})
}
