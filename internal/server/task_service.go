package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"grns/internal/api"
	"grns/internal/models"
	"grns/internal/store"
)

// TaskService centralizes task business rules, validation, and orchestration.
type TaskService struct {
	store         store.TaskServiceStore
	projectPrefix string
	importer      *Importer
}

// NewTaskService constructs a TaskService.
func NewTaskService(store store.TaskServiceStore, projectPrefix string) *TaskService {
	return &TaskService{
		store:         store,
		projectPrefix: projectPrefix,
		importer:      NewImporter(store),
	}
}

// Create creates a task from a request.
func (s *TaskService) Create(ctx context.Context, req api.TaskCreateRequest) (api.TaskResponse, error) {
	prefix, err := normalizePrefix(s.projectPrefix)
	if err != nil {
		return api.TaskResponse{}, err
	}

	prepared, err := s.prepareCreateRequest(prefix, req, s.store.TaskExists, time.Now().UTC())
	if err != nil {
		return api.TaskResponse{}, err
	}

	createdIDs := map[string]bool{prepared.task.ID: true}
	if err := s.validateDependencyParents(prepared.deps, createdIDs, s.store.TaskExists); err != nil {
		return api.TaskResponse{}, err
	}

	if err := s.store.CreateTask(ctx, prepared.task, prepared.labels, prepared.deps); err != nil {
		if isUniqueConstraint(err) {
			return api.TaskResponse{}, conflictCode(fmt.Errorf("id already exists"), ErrCodeTaskIDExists)
		}
		if isForeignKeyConstraint(err) {
			return api.TaskResponse{}, badRequestCode(fmt.Errorf("invalid dependency parent_id"), ErrCodeInvalidDependency)
		}
		return api.TaskResponse{}, err
	}

	return prepared.response, nil
}

// BatchCreate creates tasks in a single transaction.
func (s *TaskService) BatchCreate(ctx context.Context, reqs []api.TaskCreateRequest) ([]api.TaskResponse, error) {
	if len(reqs) == 0 {
		return nil, badRequestCode(fmt.Errorf("tasks array is required"), ErrCodeMissingRequired)
	}

	prefix, err := normalizePrefix(s.projectPrefix)
	if err != nil {
		return nil, err
	}

	reservedIDs := make(map[string]bool, len(reqs))
	taskExistsCache := make(map[string]bool)
	exists := func(id string) (bool, error) {
		if reservedIDs[id] {
			return true, nil
		}
		if cached, ok := taskExistsCache[id]; ok {
			return cached, nil
		}
		found, err := s.store.TaskExists(id)
		if err != nil {
			return false, err
		}
		taskExistsCache[id] = found
		return found, nil
	}

	existsInStore := func(id string) (bool, error) {
		if cached, ok := taskExistsCache[id]; ok {
			return cached, nil
		}
		found, err := s.store.TaskExists(id)
		if err != nil {
			return false, err
		}
		taskExistsCache[id] = found
		return found, nil
	}

	preparedBatch := make([]preparedTaskCreate, 0, len(reqs))
	for _, req := range reqs {
		prepared, err := s.prepareCreateRequest(prefix, req, exists, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		reservedIDs[prepared.task.ID] = true
		taskExistsCache[prepared.task.ID] = true
		preparedBatch = append(preparedBatch, prepared)
	}

	batch := make([]store.TaskCreateInput, 0, len(reqs))
	responses := make([]api.TaskResponse, 0, len(reqs))
	for _, prepared := range preparedBatch {
		if err := s.validateDependencyParents(prepared.deps, reservedIDs, existsInStore); err != nil {
			return nil, err
		}
		batch = append(batch, store.TaskCreateInput{Task: prepared.task, Labels: prepared.labels, Deps: prepared.deps})
		responses = append(responses, prepared.response)
	}

	if err := s.store.CreateTasks(ctx, batch); err != nil {
		if isUniqueConstraint(err) {
			return nil, conflictCode(fmt.Errorf("id already exists"), ErrCodeTaskIDExists)
		}
		if isForeignKeyConstraint(err) {
			return nil, badRequestCode(fmt.Errorf("invalid dependency parent_id"), ErrCodeInvalidDependency)
		}
		return nil, err
	}

	return responses, nil
}

type preparedTaskCreate struct {
	task     *models.Task
	labels   []string
	deps     []models.Dependency
	response api.TaskResponse
}

func (s *TaskService) prepareCreateRequest(prefix string, req api.TaskCreateRequest, exists func(string) (bool, error), now time.Time) (preparedTaskCreate, error) {
	if strings.TrimSpace(req.Title) == "" {
		return preparedTaskCreate{}, badRequestCode(fmt.Errorf("title is required"), ErrCodeMissingRequired)
	}

	status := string(models.StatusOpen)
	if req.Status != nil {
		value, err := normalizeStatus(*req.Status)
		if err != nil {
			return preparedTaskCreate{}, badRequest(err)
		}
		status = value
	}

	taskType := string(models.TypeTask)
	if req.Type != nil {
		value, err := normalizeType(*req.Type)
		if err != nil {
			return preparedTaskCreate{}, badRequest(err)
		}
		taskType = value
	}

	priority := models.DefaultPriority
	if req.Priority != nil {
		if !models.IsValidPriority(*req.Priority) {
			return preparedTaskCreate{}, badRequestCode(fmt.Errorf("priority must be between %d and %d", models.PriorityMin, models.PriorityMax), ErrCodeInvalidPriority)
		}
		priority = *req.Priority
	}

	labels, err := normalizeLabels(req.Labels)
	if err != nil {
		return preparedTaskCreate{}, badRequest(err)
	}

	id := strings.TrimSpace(req.ID)
	if id != "" {
		if !validateID(id) || !strings.HasPrefix(id, prefix+"-") {
			return preparedTaskCreate{}, badRequestCode(fmt.Errorf("invalid id"), ErrCodeInvalidID)
		}
		if exists != nil {
			found, err := exists(id)
			if err != nil {
				return preparedTaskCreate{}, err
			}
			if found {
				return preparedTaskCreate{}, conflictCode(fmt.Errorf("id already exists"), ErrCodeTaskIDExists)
			}
		}
	} else {
		id, err = store.GenerateID(prefix, exists)
		if err != nil {
			return preparedTaskCreate{}, err
		}
	}

	parentID := ""
	if req.ParentID != nil {
		parentID = strings.TrimSpace(*req.ParentID)
		if parentID != "" && !validateID(parentID) {
			return preparedTaskCreate{}, badRequestCode(fmt.Errorf("invalid parent_id"), ErrCodeInvalidParentID)
		}
	}

	deps := make([]models.Dependency, 0, len(req.Deps))
	for _, dep := range req.Deps {
		parent := strings.TrimSpace(dep.ParentID)
		if parent == "" || !validateID(parent) {
			return preparedTaskCreate{}, badRequestCode(fmt.Errorf("invalid dependency parent_id"), ErrCodeInvalidDependency)
		}
		depType := strings.TrimSpace(dep.Type)
		if depType == "" {
			depType = string(models.DependencyBlocks)
		}
		deps = append(deps, models.Dependency{ParentID: parent, Type: depType})
	}

	task := &models.Task{
		ID:                 id,
		Title:              strings.TrimSpace(req.Title),
		Status:             status,
		Type:               taskType,
		Priority:           priority,
		Description:        valueOrEmpty(req.Description),
		SpecID:             valueOrEmpty(req.SpecID),
		ParentID:           parentID,
		Assignee:           valueOrEmpty(req.Assignee),
		Notes:              valueOrEmpty(req.Notes),
		Design:             valueOrEmpty(req.Design),
		AcceptanceCriteria: valueOrEmpty(req.AcceptanceCriteria),
		SourceRepo:         valueOrEmpty(req.SourceRepo),
		Custom:             req.Custom,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if status == string(models.StatusClosed) {
		task.ClosedAt = &now
	}

	return preparedTaskCreate{
		task:     task,
		labels:   labels,
		deps:     deps,
		response: api.TaskResponse{Task: *task, Labels: labels, Deps: deps},
	}, nil
}

func (s *TaskService) validateDependencyParents(deps []models.Dependency, createdIDs map[string]bool, taskExists func(string) (bool, error)) error {
	for _, dep := range deps {
		if createdIDs != nil && createdIDs[dep.ParentID] {
			continue
		}
		if taskExists == nil {
			continue
		}
		exists, err := taskExists(dep.ParentID)
		if err != nil {
			return err
		}
		if !exists {
			return badRequestCode(fmt.Errorf("invalid dependency parent_id"), ErrCodeInvalidDependency)
		}
	}
	return nil
}

// Update updates a task and returns the updated response.
func (s *TaskService) Update(ctx context.Context, id string, req api.TaskUpdateRequest) (api.TaskResponse, error) {
	var resp api.TaskResponse

	update, err := buildTaskUpdateFromRequest(req, time.Now().UTC())
	if err != nil {
		return resp, err
	}

	if err := s.store.UpdateTask(ctx, id, update.toStoreTaskUpdate()); err != nil {
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
		return resp, notFoundCode(fmt.Errorf("task not found"), ErrCodeTaskNotFound)
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

// GetMany returns multiple task responses, preserving request order (including duplicates).
func (s *TaskService) GetMany(ctx context.Context, ids []string) ([]api.TaskResponse, error) {
	if len(ids) == 0 {
		return nil, badRequestCode(fmt.Errorf("ids are required"), ErrCodeMissingRequired)
	}

	uniqueIDs := uniqueStrings(ids)
	tasks, err := s.store.ListTasks(ctx, taskListFilter{IDs: uniqueIDs}.toStoreListFilter())
	if err != nil {
		return nil, err
	}
	if len(tasks) != len(uniqueIDs) {
		return nil, notFoundCode(fmt.Errorf("task not found"), ErrCodeTaskNotFound)
	}

	responsesByTaskOrder, err := s.attachLabelsAndDeps(ctx, tasks)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]api.TaskResponse, len(responsesByTaskOrder))
	for _, resp := range responsesByTaskOrder {
		byID[resp.Task.ID] = resp
	}

	responses := make([]api.TaskResponse, 0, len(ids))
	for _, id := range ids {
		resp, ok := byID[id]
		if !ok {
			return nil, notFoundCode(fmt.Errorf("task not found"), ErrCodeTaskNotFound)
		}
		responses = append(responses, resp)
	}

	return responses, nil
}

// List returns tasks with labels.
func (s *TaskService) List(ctx context.Context, filter taskListFilter) ([]api.TaskResponse, error) {
	tasks, err := s.store.ListTasks(ctx, filter.toStoreListFilter())
	if err != nil {
		if filter.SearchQuery != "" && isInvalidSearchQuery(err) {
			return nil, badRequestCode(fmt.Errorf("invalid search query"), ErrCodeInvalidSearchQuery)
		}
		return nil, err
	}
	return s.attachLabels(ctx, tasks)
}

// ExportPage returns one export page hydrated with labels and dependencies.
func (s *TaskService) ExportPage(ctx context.Context, limit, offset int) ([]api.TaskResponse, error) {
	tasks, err := s.store.ListTasks(ctx, taskListFilter{Limit: limit, Offset: offset}.toStoreListFilter())
	if err != nil {
		return nil, err
	}
	return s.attachLabelsAndDeps(ctx, tasks)
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
	err := s.store.CloseTasks(ctx, ids, now)
	if errors.Is(err, store.ErrTaskNotFound) {
		return notFoundCode(fmt.Errorf("task not found"), ErrCodeTaskNotFound)
	}
	return err
}

// CloseWithCommit closes tasks and atomically records closed_by git refs for each task.
func (s *TaskService) CloseWithCommit(ctx context.Context, ids []string, commit, repo string) (int, error) {
	ids = uniqueStrings(ids)
	if len(ids) == 0 {
		return 0, badRequestCode(fmt.Errorf("ids are required"), ErrCodeMissingRequired)
	}
	if strings.TrimSpace(commit) == "" {
		return 0, badRequestCode(fmt.Errorf("commit is required"), ErrCodeMissingRequired)
	}

	gitRefStore, ok := any(s.store).(store.GitRefStore)
	if !ok {
		return 0, internalError(fmt.Errorf("git refs are not configured"))
	}

	canonicalRepo := ""
	var err error
	if strings.TrimSpace(repo) != "" {
		canonicalRepo, err = canonicalGitRepoSlug(repo)
		if err != nil {
			return 0, closeRepoValidationError(err)
		}
	}

	refs := make([]store.CloseTaskGitRefInput, 0, len(ids))
	for _, id := range ids {
		task, err := s.store.GetTask(ctx, id)
		if err != nil {
			return 0, err
		}
		if task == nil {
			return 0, notFoundCode(fmt.Errorf("task not found"), ErrCodeTaskNotFound)
		}

		repoSlug := canonicalRepo
		if repoSlug == "" {
			repoSlug, err = canonicalGitRepoSlug(task.SourceRepo)
			if err != nil {
				return 0, closeRepoValidationError(err)
			}
		}

		refs = append(refs, store.CloseTaskGitRefInput{
			TaskID:      id,
			RepoSlug:    repoSlug,
			Relation:    "closed_by",
			ObjectType:  string(models.GitObjectTypeCommit),
			ObjectValue: commit,
		})
	}

	now := time.Now().UTC()
	created, err := gitRefStore.CloseTasksWithGitRefs(ctx, ids, now, refs)
	if errors.Is(err, store.ErrTaskNotFound) {
		return 0, notFoundCode(fmt.Errorf("task not found"), ErrCodeTaskNotFound)
	}
	if err != nil {
		return 0, err
	}
	return created, nil
}

func closeRepoValidationError(err error) error {
	code := ErrCodeInvalidArgument
	if strings.Contains(err.Error(), "required") {
		code = ErrCodeMissingRequired
	}
	return badRequestCode(err, code)
}

// Reopen reopens tasks by ids.
func (s *TaskService) Reopen(ctx context.Context, ids []string) error {
	now := time.Now().UTC()
	err := s.store.ReopenTasks(ctx, ids, now)
	if errors.Is(err, store.ErrTaskNotFound) {
		return notFoundCode(fmt.Errorf("task not found"), ErrCodeTaskNotFound)
	}
	return err
}

// AddDependency adds a dependency edge between tasks.
func (s *TaskService) AddDependency(ctx context.Context, childID, parentID, depType string) error {
	if !validateID(childID) || !validateID(parentID) {
		return badRequestCode(fmt.Errorf("invalid dependency ids"), ErrCodeInvalidDependency)
	}
	if err := s.ensureTaskExists(childID); err != nil {
		return err
	}
	if err := s.ensureTaskExists(parentID); err != nil {
		return err
	}
	depType = strings.TrimSpace(depType)
	if depType == "" {
		depType = string(models.DependencyBlocks)
	}
	return s.store.AddDependency(ctx, childID, parentID, depType)
}

// AddLabels adds labels to a task and returns the updated label set.
func (s *TaskService) AddLabels(ctx context.Context, id string, labels []string) ([]string, error) {
	if !validateID(id) {
		return nil, badRequestCode(fmt.Errorf("invalid id"), ErrCodeInvalidID)
	}
	if err := s.ensureTaskExists(id); err != nil {
		return nil, err
	}
	normalized, err := normalizeLabels(labels)
	if err != nil {
		return nil, badRequest(err)
	}
	if err := s.store.AddLabels(ctx, id, normalized); err != nil {
		return nil, err
	}
	return s.store.ListLabels(ctx, id)
}

// RemoveLabels removes labels from a task and returns the updated label set.
func (s *TaskService) RemoveLabels(ctx context.Context, id string, labels []string) ([]string, error) {
	if !validateID(id) {
		return nil, badRequestCode(fmt.Errorf("invalid id"), ErrCodeInvalidID)
	}
	if err := s.ensureTaskExists(id); err != nil {
		return nil, err
	}
	normalized, err := normalizeLabels(labels)
	if err != nil {
		return nil, badRequest(err)
	}
	if err := s.store.RemoveLabels(ctx, id, normalized); err != nil {
		return nil, err
	}
	return s.store.ListLabels(ctx, id)
}

// Import processes an import request.
func (s *TaskService) Import(ctx context.Context, req api.ImportRequest) (api.ImportResponse, error) {
	return s.importer.Import(ctx, req)
}

func (s *TaskService) ensureTaskExists(id string) error {
	exists, err := s.store.TaskExists(id)
	if err != nil {
		return err
	}
	if !exists {
		return notFoundCode(fmt.Errorf("task not found"), ErrCodeTaskNotFound)
	}
	return nil
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (s *TaskService) attachLabels(ctx context.Context, tasks []models.Task) ([]api.TaskResponse, error) {
	labelMap, err := s.store.ListLabelsForTasks(ctx, taskIDs(tasks))
	if err != nil {
		return nil, err
	}
	return mapTaskResponses(tasks, labelMap, nil), nil
}

func (s *TaskService) attachLabelsAndDeps(ctx context.Context, tasks []models.Task) ([]api.TaskResponse, error) {
	ids := taskIDs(tasks)
	labelMap, err := s.store.ListLabelsForTasks(ctx, ids)
	if err != nil {
		return nil, err
	}
	depMap, err := s.store.ListDependenciesForTasks(ctx, ids)
	if err != nil {
		return nil, err
	}
	return mapTaskResponses(tasks, labelMap, depMap), nil
}

func taskIDs(tasks []models.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func mapTaskResponses(tasks []models.Task, labelMap map[string][]string, depMap map[string][]models.Dependency) []api.TaskResponse {
	responses := make([]api.TaskResponse, 0, len(tasks))
	for _, task := range tasks {
		labels := labelMap[task.ID]
		if labels == nil {
			labels = []string{}
		}
		resp := api.TaskResponse{Task: task, Labels: labels}
		if depMap != nil {
			resp.Deps = depMap[task.ID]
		}
		responses = append(responses, resp)
	}
	return responses
}
