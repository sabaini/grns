package server

import (
	"context"
	"testing"
	"time"

	"grns/internal/api"
	"grns/internal/models"
)

func TestTaskService_ProjectContextValidation(t *testing.T) {
	svc, _ := newTaskServiceForTest(t)
	ctxGR := contextWithProject(context.Background(), "gr")
	ctxXY := contextWithProject(context.Background(), "xy")

	if _, err := svc.Create(ctxGR, api.TaskCreateRequest{ID: "gr-p511", Title: "gr task"}); err != nil {
		t.Fatalf("create gr task: %v", err)
	}
	if _, err := svc.Create(ctxXY, api.TaskCreateRequest{ID: "xy-p511", Title: "xy task"}); err != nil {
		t.Fatalf("create xy task: %v", err)
	}

	if _, err := svc.Get(ctxGR, "xy-p511"); httpStatusFromError(err) != 404 {
		t.Fatalf("expected 404 for cross-project get, got status=%d err=%v", httpStatusFromError(err), err)
	}

	if err := svc.AddDependency(ctxGR, "gr-p511", "xy-p511", "blocks"); httpStatusFromError(err) != 400 {
		t.Fatalf("expected 400 for cross-project dependency, got status=%d err=%v", httpStatusFromError(err), err)
	}

	if _, err := svc.Create(ctxXY, api.TaskCreateRequest{ID: "gr-p512", Title: "wrong prefix"}); httpStatusFromError(err) != 400 {
		t.Fatalf("expected 400 for id/project mismatch, got status=%d err=%v", httpStatusFromError(err), err)
	}
}

func TestAttachmentService_ProjectContextValidation(t *testing.T) {
	svc, st := newAttachmentServiceForTest(t)
	now := time.Now().UTC()

	ctxGR := contextWithProject(context.Background(), "gr")
	ctxXY := contextWithProject(context.Background(), "xy")

	for _, task := range []*models.Task{
		{ID: "gr-a511", Title: "gr task", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "xy-a511", Title: "xy task", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now},
	} {
		if err := st.CreateTask(context.Background(), task, nil, nil); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	created, err := svc.CreateLinkAttachment(ctxXY, "xy-a511", CreateLinkAttachmentInput{
		Kind:        string(models.AttachmentKindArtifact),
		ExternalURL: "https://example.com/xy",
	})
	if err != nil {
		t.Fatalf("create xy attachment: %v", err)
	}

	if _, err := svc.GetAttachment(ctxGR, created.ID); httpStatusFromError(err) != 404 {
		t.Fatalf("expected 404 for cross-project attachment get, got status=%d err=%v", httpStatusFromError(err), err)
	}

	if err := svc.DeleteAttachment(ctxGR, created.ID); httpStatusFromError(err) != 404 {
		t.Fatalf("expected 404 for cross-project attachment delete, got status=%d err=%v", httpStatusFromError(err), err)
	}

	if _, err := svc.GetAttachment(ctxXY, created.ID); err != nil {
		t.Fatalf("expected attachment to remain accessible in owning project: %v", err)
	}

	if _, err := svc.CreateLinkAttachment(ctxGR, "xy-a511", CreateLinkAttachmentInput{
		Kind:        string(models.AttachmentKindArtifact),
		ExternalURL: "https://example.com/xy-2",
	}); httpStatusFromError(err) != 404 {
		t.Fatalf("expected 404 for cross-project task attachment create, got status=%d err=%v", httpStatusFromError(err), err)
	}
}

func TestTaskGitRefService_ProjectContextValidation(t *testing.T) {
	_, st := newTaskServiceForTest(t)
	now := time.Now().UTC()

	for _, task := range []*models.Task{
		{ID: "gr-g511", Title: "gr task", Status: "open", Type: "task", Priority: 2, SourceRepo: "github.com/acme/repo", CreatedAt: now, UpdatedAt: now},
		{ID: "xy-g511", Title: "xy task", Status: "open", Type: "task", Priority: 2, SourceRepo: "github.com/acme/repo", CreatedAt: now, UpdatedAt: now},
	} {
		if err := st.CreateTask(context.Background(), task, nil, nil); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	svc := NewTaskGitRefService(st, st, "gr")
	ctxGR := contextWithProject(context.Background(), "gr")
	ctxXY := contextWithProject(context.Background(), "xy")

	created, err := svc.Create(ctxXY, "xy-g511", api.TaskGitRefCreateRequest{
		Relation:    "related",
		ObjectType:  "path",
		ObjectValue: "docs/design.md",
	})
	if err != nil {
		t.Fatalf("create xy git ref: %v", err)
	}

	if _, err := svc.Get(ctxGR, created.ID); httpStatusFromError(err) != 404 {
		t.Fatalf("expected 404 for cross-project git ref get, got status=%d err=%v", httpStatusFromError(err), err)
	}

	if err := svc.Delete(ctxGR, created.ID); httpStatusFromError(err) != 404 {
		t.Fatalf("expected 404 for cross-project git ref delete, got status=%d err=%v", httpStatusFromError(err), err)
	}

	if _, err := svc.Get(ctxXY, created.ID); err != nil {
		t.Fatalf("expected git ref to remain accessible in owning project: %v", err)
	}

	if _, err := svc.Create(ctxXY, "gr-g511", api.TaskGitRefCreateRequest{
		Relation:    "related",
		ObjectType:  "path",
		ObjectValue: "docs/other.md",
	}); httpStatusFromError(err) != 404 {
		t.Fatalf("expected 404 for cross-project task git-ref create, got status=%d err=%v", httpStatusFromError(err), err)
	}
}
