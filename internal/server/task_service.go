package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"grns/internal/api"
	"grns/internal/models"
	"grns/internal/store"
)

// TaskService centralizes task validation and defaults.
type TaskService struct {
	store         store.TaskStore
	projectPrefix string
}

// NewTaskService constructs a TaskService.
func NewTaskService(store store.TaskStore, projectPrefix string) *TaskService {
	return &TaskService{store: store, projectPrefix: projectPrefix}
}

// Create creates a task from a request.
func (s *TaskService) Create(ctx context.Context, req api.TaskCreateRequest) (api.TaskResponse, error) {
	var resp api.TaskResponse

	if strings.TrimSpace(req.Title) == "" {
		return resp, badRequest(fmt.Errorf("title is required"))
	}

	prefix, err := normalizePrefix(s.projectPrefix)
	if err != nil {
		return resp, err
	}

	status := defaultStatus
	if req.Status != nil {
		status, err = normalizeStatus(*req.Status)
		if err != nil {
			return resp, badRequest(err)
		}
	}

	issueType := defaultType
	if req.Type != nil {
		issueType, err = normalizeType(*req.Type)
		if err != nil {
			return resp, badRequest(err)
		}
	}

	priority := defaultPriority
	if req.Priority != nil {
		if *req.Priority < 0 || *req.Priority > 4 {
			return resp, badRequest(fmt.Errorf("priority must be between 0 and 4"))
		}
		priority = *req.Priority
	}

	labels, err := normalizeLabels(req.Labels)
	if err != nil {
		return resp, badRequest(err)
	}

	id := strings.TrimSpace(req.ID)
	if id != "" {
		if !validateID(id) || !strings.HasPrefix(id, prefix+"-") {
			return resp, badRequest(fmt.Errorf("invalid id"))
		}
		exists, err := s.store.TaskExists(id)
		if err != nil {
			return resp, err
		}
		if exists {
			return resp, conflict(fmt.Errorf("id already exists"))
		}
	} else {
		id, err = store.GenerateID(prefix, s.store.TaskExists)
		if err != nil {
			return resp, err
		}
	}

	parentID := ""
	if req.ParentID != nil {
		parentID = strings.TrimSpace(*req.ParentID)
		if parentID != "" && !validateID(parentID) {
			return resp, badRequest(fmt.Errorf("invalid parent_id"))
		}
	}

	deps := make([]models.Dependency, 0, len(req.Deps))
	for _, dep := range req.Deps {
		parent := strings.TrimSpace(dep.ParentID)
		if parent == "" || !validateID(parent) {
			return resp, badRequest(fmt.Errorf("invalid dependency parent_id"))
		}
		depType := strings.TrimSpace(dep.Type)
		if depType == "" {
			depType = "blocks"
		}
		deps = append(deps, models.Dependency{ParentID: parent, Type: depType})
	}

	now := time.Now().UTC()
	task := &models.Task{
		ID:          id,
		Title:       strings.TrimSpace(req.Title),
		Status:      status,
		Type:        issueType,
		Priority:    priority,
		Description:        valueOrEmpty(req.Description),
		SpecID:             valueOrEmpty(req.SpecID),
		ParentID:           parentID,
		Assignee:           valueOrEmpty(req.Assignee),
		Notes:              valueOrEmpty(req.Notes),
		Design:             valueOrEmpty(req.Design),
		AcceptanceCriteria: valueOrEmpty(req.AcceptanceCriteria),
		SourceRepo:         valueOrEmpty(req.SourceRepo),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if status == "closed" {
		task.ClosedAt = &now
	}

	if err := s.store.CreateTask(ctx, task, labels, deps); err != nil {
		if isUniqueConstraint(err) {
			return resp, conflict(fmt.Errorf("id already exists"))
		}
		return resp, err
	}

	resp = api.TaskResponse{Task: *task, Labels: labels, Deps: deps}
	return resp, nil
}

// Update updates a task and returns the updated response.
func (s *TaskService) Update(ctx context.Context, id string, req api.TaskUpdateRequest) (api.TaskResponse, error) {
	var resp api.TaskResponse

	update := store.TaskUpdate{UpdatedAt: time.Now().UTC()}
	if req.Title != nil {
		trimmed := strings.TrimSpace(*req.Title)
		if trimmed == "" {
			return resp, badRequest(fmt.Errorf("title cannot be empty"))
		}
		update.Title = &trimmed
	}
	if req.Status != nil {
		status, err := normalizeStatus(*req.Status)
		if err != nil {
			return resp, badRequest(err)
		}
		update.Status = &status
		if status == "closed" {
			closedAt := update.UpdatedAt
			update.ClosedAt = &closedAt
		} else {
			zero := time.Time{}
			update.ClosedAt = &zero
		}
	}
	if req.Type != nil {
		issueType, err := normalizeType(*req.Type)
		if err != nil {
			return resp, badRequest(err)
		}
		update.Type = &issueType
	}
	if req.Priority != nil {
		if *req.Priority < 0 || *req.Priority > 4 {
			return resp, badRequest(fmt.Errorf("priority must be between 0 and 4"))
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
			return resp, badRequest(fmt.Errorf("invalid parent_id"))
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

	if err := s.store.UpdateTask(ctx, id, update); err != nil {
		return resp, err
	}

	return s.Get(ctx, id)
}

// Get returns a task response by id.
func (s *TaskService) Get(ctx context.Context, id string) (api.TaskResponse, error) {
	var resp api.TaskResponse

	task, err := s.store.GetTask(ctx, id)
	if err != nil {
		return resp, err
	}
	if task == nil {
		return resp, notFound(fmt.Errorf("task not found"))
	}

	labels, err := s.store.ListLabels(ctx, id)
	if err != nil {
		return resp, err
	}
	deps, err := s.store.ListDependencies(ctx, id)
	if err != nil {
		return resp, err
	}

	resp = api.TaskResponse{Task: *task, Labels: labels, Deps: deps}
	return resp, nil
}

// List returns tasks with labels.
func (s *TaskService) List(ctx context.Context, filter store.ListFilter) ([]api.TaskResponse, error) {
	tasks, err := s.store.ListTasks(ctx, filter)
	if err != nil {
		return nil, err
	}
	return s.attachLabels(ctx, tasks)
}

// Ready returns ready tasks with labels.
func (s *TaskService) Ready(ctx context.Context, limit int) ([]api.TaskResponse, error) {
	tasks, err := s.store.ListReadyTasks(ctx, limit)
	if err != nil {
		return nil, err
	}
	return s.attachLabels(ctx, tasks)
}

// Stale returns stale tasks with labels.
func (s *TaskService) Stale(ctx context.Context, cutoff time.Time, statuses []string, limit int) ([]api.TaskResponse, error) {
	tasks, err := s.store.ListStaleTasks(ctx, cutoff, statuses, limit)
	if err != nil {
		return nil, err
	}
	return s.attachLabels(ctx, tasks)
}

// Close closes tasks by ids.
func (s *TaskService) Close(ctx context.Context, ids []string) error {
	now := time.Now().UTC()
	return s.store.CloseTasks(ctx, ids, now)
}

// Reopen reopens tasks by ids.
func (s *TaskService) Reopen(ctx context.Context, ids []string) error {
	now := time.Now().UTC()
	return s.store.ReopenTasks(ctx, ids, now)
}

func (s *TaskService) attachLabels(ctx context.Context, tasks []models.Task) ([]api.TaskResponse, error) {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}

	labelMap, err := s.store.ListLabelsForTasks(ctx, ids)
	if err != nil {
		return nil, err
	}

	responses := make([]api.TaskResponse, 0, len(tasks))
	for _, task := range tasks {
		labels := labelMap[task.ID]
		if labels == nil {
			labels = []string{}
		}
		responses = append(responses, api.TaskResponse{
			Task:   task,
			Labels: labels,
		})
	}

	return responses, nil
}
