package server

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"grns/internal/api"
	"grns/internal/models"
	"grns/internal/store"
)

var (
	gitHashRegex        = regexp.MustCompile(`^[0-9a-f]{40}$`)
	gitRelationRegex    = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	gitBuiltinRelations = map[string]struct{}{
		"design_doc":    {},
		"implements":    {},
		"fix_commit":    {},
		"closed_by":     {},
		"introduced_by": {},
		"related":       {},
	}
)

// TaskGitRefService orchestrates task↔git reference workflows.
type TaskGitRefService struct {
	taskStore   store.TaskServiceStore
	gitRefStore store.GitRefStore
}

// NewTaskGitRefService constructs a TaskGitRefService.
func NewTaskGitRefService(taskStore store.TaskServiceStore, gitRefStore store.GitRefStore) *TaskGitRefService {
	return &TaskGitRefService{taskStore: taskStore, gitRefStore: gitRefStore}
}

// Create creates one task git ref.
func (s *TaskGitRefService) Create(ctx context.Context, taskID string, req api.TaskGitRefCreateRequest) (models.TaskGitRef, error) {
	var zero models.TaskGitRef
	if s == nil || s.taskStore == nil || s.gitRefStore == nil {
		return zero, internalError(fmt.Errorf("task git ref service is not configured"))
	}

	taskID = strings.TrimSpace(taskID)
	if !validateID(taskID) {
		return zero, badRequestCode(fmt.Errorf("invalid task_id"), ErrCodeInvalidID)
	}

	task, err := s.taskStore.GetTask(ctx, taskID)
	if err != nil {
		return zero, err
	}
	if task == nil {
		return zero, notFoundCode(fmt.Errorf("task not found"), ErrCodeTaskNotFound)
	}

	relation, err := normalizeGitRelation(req.Relation)
	if err != nil {
		return zero, err
	}
	objectType, err := models.ParseGitObjectType(req.ObjectType)
	if err != nil {
		return zero, badRequestCode(err, ErrCodeInvalidArgument)
	}
	objectValue, err := normalizeGitObjectValue(objectType, req.ObjectValue)
	if err != nil {
		return zero, err
	}
	resolvedCommit, err := normalizeGitHash(req.ResolvedCommit, "resolved_commit")
	if err != nil {
		return zero, err
	}

	repoInput := strings.TrimSpace(req.Repo)
	if repoInput == "" {
		repoInput = strings.TrimSpace(task.SourceRepo)
	}
	repoSlug, err := canonicalGitRepoSlug(repoInput)
	if err != nil {
		code := ErrCodeInvalidArgument
		if strings.Contains(err.Error(), "required") {
			code = ErrCodeMissingRequired
		}
		return zero, badRequestCode(err, code)
	}

	repo, err := s.gitRefStore.UpsertGitRepo(ctx, &models.GitRepo{Slug: repoSlug})
	if err != nil {
		return zero, err
	}
	if repo == nil || !validateGitRepoID(repo.ID) {
		return zero, internalError(fmt.Errorf("invalid git repo state"))
	}

	id, err := s.nextTaskGitRefID(ctx)
	if err != nil {
		return zero, err
	}
	now := time.Now().UTC()

	ref := &models.TaskGitRef{
		ID:             id,
		TaskID:         taskID,
		RepoID:         repo.ID,
		Repo:           repo.Slug,
		Relation:       relation,
		ObjectType:     string(objectType),
		ObjectValue:    objectValue,
		ResolvedCommit: resolvedCommit,
		Note:           strings.TrimSpace(req.Note),
		Meta:           req.Meta,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.gitRefStore.CreateTaskGitRef(ctx, ref); err != nil {
		if isUniqueConstraint(err) {
			return zero, conflictCode(fmt.Errorf("git ref already exists"), ErrCodeConflict)
		}
		return zero, err
	}

	stored, err := s.gitRefStore.GetTaskGitRef(ctx, ref.ID)
	if err != nil {
		return zero, err
	}
	if stored == nil {
		return zero, internalError(fmt.Errorf("git ref not found after create"))
	}
	return *stored, nil
}

// List lists git refs for one task.
func (s *TaskGitRefService) List(ctx context.Context, taskID string) ([]models.TaskGitRef, error) {
	if s == nil || s.taskStore == nil || s.gitRefStore == nil {
		return nil, internalError(fmt.Errorf("task git ref service is not configured"))
	}

	taskID = strings.TrimSpace(taskID)
	if !validateID(taskID) {
		return nil, badRequestCode(fmt.Errorf("invalid task_id"), ErrCodeInvalidID)
	}
	if err := s.ensureTaskExists(taskID); err != nil {
		return nil, err
	}

	return s.gitRefStore.ListTaskGitRefs(ctx, taskID)
}

// Get returns one task git ref by id.
func (s *TaskGitRefService) Get(ctx context.Context, id string) (models.TaskGitRef, error) {
	var zero models.TaskGitRef
	if s == nil || s.gitRefStore == nil {
		return zero, internalError(fmt.Errorf("task git ref service is not configured"))
	}

	id = strings.TrimSpace(id)
	if !validateGitRefID(id) {
		return zero, badRequestCode(fmt.Errorf("invalid ref_id"), ErrCodeInvalidID)
	}

	ref, err := s.gitRefStore.GetTaskGitRef(ctx, id)
	if err != nil {
		return zero, err
	}
	if ref == nil {
		return zero, notFoundCode(fmt.Errorf("git ref not found"), ErrCodeGitRefNotFound)
	}
	return *ref, nil
}

// Delete deletes one task git ref by id.
func (s *TaskGitRefService) Delete(ctx context.Context, id string) error {
	if s == nil || s.gitRefStore == nil {
		return internalError(fmt.Errorf("task git ref service is not configured"))
	}

	id = strings.TrimSpace(id)
	if !validateGitRefID(id) {
		return badRequestCode(fmt.Errorf("invalid ref_id"), ErrCodeInvalidID)
	}

	ref, err := s.gitRefStore.GetTaskGitRef(ctx, id)
	if err != nil {
		return err
	}
	if ref == nil {
		return notFoundCode(fmt.Errorf("git ref not found"), ErrCodeGitRefNotFound)
	}

	return s.gitRefStore.DeleteTaskGitRef(ctx, id)
}

func (s *TaskGitRefService) ensureTaskExists(id string) error {
	exists, err := s.taskStore.TaskExists(id)
	if err != nil {
		return err
	}
	if !exists {
		return notFoundCode(fmt.Errorf("task not found"), ErrCodeTaskNotFound)
	}
	return nil
}

func (s *TaskGitRefService) nextTaskGitRefID(ctx context.Context) (string, error) {
	exists := func(id string) (bool, error) {
		ref, err := s.gitRefStore.GetTaskGitRef(ctx, id)
		if err != nil {
			return false, err
		}
		return ref != nil, nil
	}
	return store.GenerateTaskGitRefID(exists)
}

func normalizeGitRelation(raw string) (string, error) {
	relation := strings.ToLower(strings.TrimSpace(raw))
	if relation == "" {
		return "", badRequestCode(fmt.Errorf("relation is required"), ErrCodeMissingRequired)
	}
	if !gitRelationRegex.MatchString(relation) {
		return "", badRequestCode(fmt.Errorf("invalid relation"), ErrCodeInvalidArgument)
	}
	if _, ok := gitBuiltinRelations[relation]; ok {
		return relation, nil
	}
	if strings.HasPrefix(relation, "x-") {
		return relation, nil
	}
	return "", badRequestCode(fmt.Errorf("invalid relation"), ErrCodeInvalidArgument)
}

func normalizeGitObjectValue(objectType models.GitObjectType, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", badRequestCode(fmt.Errorf("object_value is required"), ErrCodeMissingRequired)
	}

	switch objectType {
	case models.GitObjectTypeCommit, models.GitObjectTypeBlob, models.GitObjectTypeTree:
		return normalizeGitHash(value, "object_value")
	case models.GitObjectTypePath:
		if err := validateWorkspaceRelativePath(value); err != nil {
			return "", badRequestCode(fmt.Errorf("invalid object_value path"), ErrCodeInvalidArgument)
		}
		clean := filepath.ToSlash(filepath.Clean(value))
		if clean == "." {
			return "", badRequestCode(fmt.Errorf("invalid object_value path"), ErrCodeInvalidArgument)
		}
		return clean, nil
	case models.GitObjectTypeBranch, models.GitObjectTypeTag:
		if strings.ContainsAny(value, "\t\n\r ") {
			return "", badRequestCode(fmt.Errorf("object_value must not contain whitespace"), ErrCodeInvalidArgument)
		}
		return value, nil
	default:
		return "", badRequestCode(fmt.Errorf("invalid object_type"), ErrCodeInvalidArgument)
	}
}

func normalizeGitHash(raw, field string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", nil
	}
	if !gitHashRegex.MatchString(value) {
		return "", badRequestCode(fmt.Errorf("%s must be a 40-char lowercase hex git hash", field), ErrCodeInvalidArgument)
	}
	return value, nil
}

func canonicalGitRepoSlug(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("repo is required (or task.source_repo must be set)")
	}

	if strings.Contains(value, "://") {
		u, err := url.Parse(value)
		if err != nil || strings.TrimSpace(u.Host) == "" {
			return "", fmt.Errorf("invalid repo")
		}
		value = strings.TrimSpace(u.Host) + "/" + strings.Trim(strings.TrimSpace(u.Path), "/")
	} else if strings.Contains(value, "@") && strings.Contains(value, ":") {
		parts := strings.SplitN(value, ":", 2)
		hostPart := parts[0]
		host := hostPart
		if at := strings.LastIndex(hostPart, "@"); at >= 0 {
			host = hostPart[at+1:]
		}
		value = strings.TrimSpace(host) + "/" + strings.Trim(strings.TrimSpace(parts[1]), "/")
	}

	value = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".git")
	value = strings.Trim(value, "/")
	parts := strings.Split(value, "/")
	if len(parts) != 3 {
		return "", fmt.Errorf("repo must be host/owner/name")
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || strings.ContainsAny(part, "\t\n\r ") {
			return "", fmt.Errorf("repo must be host/owner/name")
		}
	}
	return strings.Join(parts, "/"), nil
}
