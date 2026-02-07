package server

import (
	"fmt"
	"strings"
	"time"

	"grns/internal/api"
	"grns/internal/models"
)

// buildTaskUpdateFromRequest maps an API update request to a service patch model.
func buildTaskUpdateFromRequest(req api.TaskUpdateRequest, updatedAt time.Time) (taskUpdatePatch, error) {
	update := taskUpdatePatch{UpdatedAt: updatedAt}

	if req.Title != nil {
		trimmed := strings.TrimSpace(*req.Title)
		if trimmed == "" {
			return taskUpdatePatch{}, badRequestCode(fmt.Errorf("title cannot be empty"), ErrCodeMissingRequired)
		}
		update.Title = &trimmed
	}
	if req.Status != nil {
		status, err := normalizeStatus(*req.Status)
		if err != nil {
			return taskUpdatePatch{}, badRequest(err)
		}
		update.Status = &status
		if status == string(models.StatusClosed) {
			closedAt := updatedAt
			update.ClosedAt = &closedAt
		} else {
			zero := time.Time{}
			update.ClosedAt = &zero
		}
	}
	if req.Type != nil {
		taskType, err := normalizeType(*req.Type)
		if err != nil {
			return taskUpdatePatch{}, badRequest(err)
		}
		update.Type = &taskType
	}
	if req.Priority != nil {
		if !models.IsValidPriority(*req.Priority) {
			return taskUpdatePatch{}, badRequestCode(fmt.Errorf("priority must be between %d and %d", models.PriorityMin, models.PriorityMax), ErrCodeInvalidPriority)
		}
		update.Priority = req.Priority
	}
	if req.Description != nil {
		update.Description = req.Description
	}
	if req.SpecID != nil {
		update.SpecID = req.SpecID
	}
	if req.ParentID != nil {
		parent := strings.TrimSpace(*req.ParentID)
		if parent != "" && !validateID(parent) {
			return taskUpdatePatch{}, badRequestCode(fmt.Errorf("invalid parent_id"), ErrCodeInvalidParentID)
		}
		update.ParentID = &parent
	}
	if req.Assignee != nil {
		update.Assignee = req.Assignee
	}
	if req.Notes != nil {
		update.Notes = req.Notes
	}
	if req.Design != nil {
		update.Design = req.Design
	}
	if req.AcceptanceCriteria != nil {
		update.AcceptanceCriteria = req.AcceptanceCriteria
	}
	if req.SourceRepo != nil {
		update.SourceRepo = req.SourceRepo
	}
	if req.Custom != nil {
		custom := req.Custom
		update.Custom = &custom
	}

	return update, nil
}

// buildTaskUpdateFromImport maps a normalized import record to a service patch model.
func buildTaskUpdateFromImport(rec api.TaskImportRecord) taskUpdatePatch {
	title := rec.Title
	status := rec.Status
	taskType := rec.Type
	priority := rec.Priority
	description := rec.Description
	specID := rec.SpecID
	parentID := rec.ParentID
	assignee := rec.Assignee
	notes := rec.Notes
	design := rec.Design
	acceptanceCriteria := rec.AcceptanceCriteria
	sourceRepo := rec.SourceRepo

	update := taskUpdatePatch{
		Title:              &title,
		Status:             &status,
		Type:               &taskType,
		Priority:           &priority,
		Description:        &description,
		SpecID:             &specID,
		ParentID:           &parentID,
		Assignee:           &assignee,
		Notes:              &notes,
		Design:             &design,
		AcceptanceCriteria: &acceptanceCriteria,
		SourceRepo:         &sourceRepo,
		UpdatedAt:          rec.UpdatedAt,
	}

	if rec.Status == string(models.StatusClosed) {
		closedAt := rec.UpdatedAt
		if rec.ClosedAt != nil {
			closedAt = rec.ClosedAt.UTC()
		}
		update.ClosedAt = &closedAt
	} else {
		zero := time.Time{}
		update.ClosedAt = &zero
	}

	if rec.Custom != nil {
		custom := rec.Custom
		update.Custom = &custom
	}

	return update
}
