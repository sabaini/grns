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

	_, err := st.CloseTasksWithGitRefs(ctx, []string{task.ID}, now, []CloseTaskGitRefInput{
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

	refs, err := st.ListTaskGitRefs(ctx, task.ID)
	if err != nil {
		t.Fatalf("list refs: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected no refs after rollback, got %d", len(refs))
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

	created, err := st.CloseTasksWithGitRefs(ctx, []string{task.ID}, now, []CloseTaskGitRefInput{ref})
	if err != nil {
		t.Fatalf("first close with git refs: %v", err)
	}
	if created != 1 {
		t.Fatalf("expected created=1, got %d", created)
	}

	created, err = st.CloseTasksWithGitRefs(ctx, []string{task.ID}, now.Add(time.Second), []CloseTaskGitRefInput{ref})
	if err != nil {
		t.Fatalf("second close with duplicate git refs: %v", err)
	}
	if created != 0 {
		t.Fatalf("expected created=0 for duplicate annotation, got %d", created)
	}

	refs, err := st.ListTaskGitRefs(ctx, task.ID)
	if err != nil {
		t.Fatalf("list refs: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected exactly one stored ref, got %d", len(refs))
	}
}
