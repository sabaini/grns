package server

import (
	"testing"
	"time"

	"grns/internal/api"
)

func TestBuildTaskUpdateFromRequest(t *testing.T) {
	t.Run("maps and normalizes fields", func(t *testing.T) {
		now := time.Date(2026, 2, 7, 9, 0, 0, 0, time.UTC)
		title := "  Updated title  "
		status := "closed"
		taskType := "bug"
		priority := 4
		parentID := " gr-ab12 "
		description := "desc"

		update, err := buildTaskUpdateFromRequest(api.TaskUpdateRequest{
			Title:       &title,
			Status:      &status,
			Type:        &taskType,
			Priority:    &priority,
			ParentID:    &parentID,
			Description: &description,
		}, now)
		if err != nil {
			t.Fatalf("build update: %v", err)
		}

		if update.Title == nil || *update.Title != "Updated title" {
			t.Fatalf("expected trimmed title, got %#v", update.Title)
		}
		if update.Status == nil || *update.Status != "closed" {
			t.Fatalf("expected closed status, got %#v", update.Status)
		}
		if update.Type == nil || *update.Type != "bug" {
			t.Fatalf("expected bug type, got %#v", update.Type)
		}
		if update.Priority == nil || *update.Priority != 4 {
			t.Fatalf("expected priority 4, got %#v", update.Priority)
		}
		if update.ParentID == nil || *update.ParentID != "gr-ab12" {
			t.Fatalf("expected normalized parent id, got %#v", update.ParentID)
		}
		if update.ClosedAt == nil || !update.ClosedAt.Equal(now) {
			t.Fatalf("expected closed_at=%s, got %#v", now, update.ClosedAt)
		}
	})

	t.Run("rejects invalid parent id", func(t *testing.T) {
		now := time.Now().UTC()
		parentID := "bad-id"
		_, err := buildTaskUpdateFromRequest(api.TaskUpdateRequest{ParentID: &parentID}, now)
		if err == nil {
			t.Fatal("expected error")
		}
		if httpStatusFromError(err) != 400 {
			t.Fatalf("expected bad request, got status %d", httpStatusFromError(err))
		}
	})
}

func TestBuildTaskUpdateFromImport(t *testing.T) {
	now := time.Date(2026, 2, 7, 10, 0, 0, 0, time.UTC)
	closedAt := now.Add(-time.Hour)
	rec := api.TaskImportRecord{}
	rec.ID = "gr-ab12"
	rec.Title = "Imported"
	rec.Status = "closed"
	rec.Type = "task"
	rec.Priority = 2
	rec.UpdatedAt = now
	rec.ClosedAt = &closedAt

	update := buildTaskUpdateFromImport(rec)
	if update.Title == nil || *update.Title != "Imported" {
		t.Fatalf("expected imported title, got %#v", update.Title)
	}
	if update.ClosedAt == nil || !update.ClosedAt.Equal(closedAt) {
		t.Fatalf("expected explicit closed_at=%s, got %#v", closedAt, update.ClosedAt)
	}

	rec.Status = "open"
	rec.ClosedAt = nil
	update = buildTaskUpdateFromImport(rec)
	if update.ClosedAt == nil || !update.ClosedAt.IsZero() {
		t.Fatalf("expected zero closed_at for open status, got %#v", update.ClosedAt)
	}
}
