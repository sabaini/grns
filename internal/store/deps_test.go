package store

import (
	"context"
	"testing"
	"time"

	"grns/internal/models"
)

func TestDependencyTree(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	for _, task := range []*models.Task{
		{ID: "gr-dt01", Title: "Task A", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "gr-dt02", Title: "Task B", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "gr-dt03", Title: "Task C", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
	} {
		if err := st.CreateTask(ctx, task, nil, nil); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}

	if err := st.AddDependency(ctx, "gr-dt01", "gr-dt02", "blocks"); err != nil {
		t.Fatalf("add dep A->B: %v", err)
	}
	if err := st.AddDependency(ctx, "gr-dt02", "gr-dt03", "blocks"); err != nil {
		t.Fatalf("add dep B->C: %v", err)
	}

	t.Run("from middle node", func(t *testing.T) {
		nodes, err := st.DependencyTree(ctx, "gr", "gr-dt02")
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
		nodes, err := st.DependencyTree(ctx, "gr", "gr-dt03")
		if err != nil {
			t.Fatalf("tree: %v", err)
		}

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

		nodes, err := st.DependencyTree(ctx, "gr", "gr-dt04")
		if err != nil {
			t.Fatalf("tree: %v", err)
		}
		if len(nodes) != 0 {
			t.Fatalf("expected 0 nodes, got %d", len(nodes))
		}
	})
}

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

func TestAddDependencyRejectsCrossProject(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	for _, task := range []*models.Task{
		{ID: "gr-cp01", Title: "gr child", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "xy-cp01", Title: "xy parent", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
	} {
		if err := st.CreateTask(ctx, task, nil, nil); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}

	err := st.AddDependency(ctx, "gr-cp01", "xy-cp01", "blocks")
	if err == nil {
		t.Fatal("expected cross-project dependency to fail")
	}
	if err != ErrProjectMismatch {
		t.Fatalf("expected ErrProjectMismatch, got %v", err)
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
