package store

import (
	"context"
	"testing"
	"time"

	"grns/internal/models"
)

func TestCloseTasksWithGitRefsRollsBackOnRefInsertError(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	task := &models.Task{
		ID:        "gr-g701",
		Title:     "atomic close rollback",
		Status:    "open",
		Type:      "task",
		Priority:  2,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create task: %v", err)
	}

	_, err := st.CloseTasksWithGitRefs(ctx, "gr", []string{task.ID}, now, []CloseTaskGitRefInput{
		{
			TaskID:      task.ID,
			RepoSlug:    "github.com/acme/repo",
			Relation:    "closed_by",
			ObjectType:  "invalid_type",
			ObjectValue: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	})
	if err == nil {
		t.Fatal("expected close with invalid git ref to fail")
	}

	storedTask, err := st.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if storedTask == nil {
		t.Fatal("expected task to exist")
	}
	if storedTask.Status != string(models.StatusOpen) {
		t.Fatalf("expected task to remain open, got %q", storedTask.Status)
	}
	if storedTask.ClosedAt != nil {
		t.Fatal("expected closed_at to remain nil after rollback")
	}

	refs, err := st.ListTaskGitRefs(ctx, "gr", task.ID)
	if err != nil {
		t.Fatalf("list refs: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected no refs after rollback, got %d", len(refs))
	}
}

func TestTaskGitRefProjectScopedByID(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, task := range []*models.Task{
		{ID: "gr-g710", Title: "gr task", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "xy-g710", Title: "xy task", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
	} {
		if err := st.CreateTask(ctx, task, nil, nil); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	repo, err := st.UpsertGitRepo(ctx, &models.GitRepo{Slug: "github.com/acme/repo"})
	if err != nil {
		t.Fatalf("upsert repo: %v", err)
	}
	if repo == nil {
		t.Fatal("expected repo")
	}

	ref := &models.TaskGitRef{
		ID:          "gf-g710",
		TaskID:      "xy-g710",
		RepoID:      repo.ID,
		Relation:    "related",
		ObjectType:  "path",
		ObjectValue: "docs/design.md",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreateTaskGitRef(ctx, ref); err != nil {
		t.Fatalf("create git ref: %v", err)
	}

	got, err := st.GetTaskGitRef(ctx, "gr", ref.ID)
	if err != nil {
		t.Fatalf("get git ref in wrong project: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil git ref for wrong project, got %#v", got)
	}

	if err := st.DeleteTaskGitRef(ctx, "gr", ref.ID); err != nil {
		t.Fatalf("delete git ref in wrong project: %v", err)
	}

	stillThere, err := st.GetTaskGitRef(ctx, "xy", ref.ID)
	if err != nil {
		t.Fatalf("get git ref in owning project: %v", err)
	}
	if stillThere == nil {
		t.Fatal("expected git ref to remain after wrong-project delete")
	}
}

func TestCloseTasksWithGitRefsIgnoresDuplicateAnnotations(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	task := &models.Task{
		ID:        "gr-g702",
		Title:     "atomic close dedupe",
		Status:    "open",
		Type:      "task",
		Priority:  2,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := st.CreateTask(ctx, task, nil, nil); err != nil {
		t.Fatalf("create task: %v", err)
	}

	ref := CloseTaskGitRefInput{
		TaskID:      task.ID,
		RepoSlug:    "github.com/acme/repo",
		Relation:    "closed_by",
		ObjectType:  "commit",
		ObjectValue: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}

	created, err := st.CloseTasksWithGitRefs(ctx, "gr", []string{task.ID}, now, []CloseTaskGitRefInput{ref})
	if err != nil {
		t.Fatalf("first close with git refs: %v", err)
	}
	if created != 1 {
		t.Fatalf("expected created=1, got %d", created)
	}

	created, err = st.CloseTasksWithGitRefs(ctx, "gr", []string{task.ID}, now.Add(time.Second), []CloseTaskGitRefInput{ref})
	if err != nil {
		t.Fatalf("second close with duplicate git refs: %v", err)
	}
	if created != 0 {
		t.Fatalf("expected created=0 for duplicate annotation, got %d", created)
	}

	refs, err := st.ListTaskGitRefs(ctx, "gr", task.ID)
	if err != nil {
		t.Fatalf("list refs: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected exactly one stored ref, got %d", len(refs))
	}
}
