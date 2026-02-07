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

// Importer executes import requests in explicit phases.
type Importer struct {
	store store.TaskStore
}

// NewImporter constructs an Importer.
func NewImporter(store store.TaskStore) *Importer {
	return &Importer{store: store}
}

type importTaskAction int

const (
	importActionNone importTaskAction = iota
	importActionCreated
	importActionUpdated
	importActionSkipped
	importActionError
)

type importRun struct {
	req             api.ImportRequest
	dedupe          string
	orphanHandling  string
	response        api.ImportResponse
	normalized      []api.TaskImportRecord
	actions         []importTaskAction
	importIDs       map[string]bool
	taskExistsCache map[string]bool
}

// Import processes an import request (validate/normalize -> upsert tasks -> apply deps).
func (i *Importer) Import(ctx context.Context, req api.ImportRequest) (api.ImportResponse, error) {
	applyMode := "best_effort"
	if req.Atomic {
		applyMode = "atomic"
	}

	run := &importRun{
		req:             req,
		dedupe:          req.Dedupe,
		orphanHandling:  req.OrphanHandling,
		response:        api.ImportResponse{DryRun: req.DryRun, TaskIDs: []string{}, ApplyMode: applyMode},
		normalized:      make([]api.TaskImportRecord, len(req.Tasks)),
		actions:         make([]importTaskAction, len(req.Tasks)),
		importIDs:       make(map[string]bool, len(req.Tasks)),
		taskExistsCache: make(map[string]bool),
	}
	if run.dedupe == "" {
		run.dedupe = "skip"
	}
	if run.orphanHandling == "" {
		run.orphanHandling = "allow"
	}

	if err := i.normalizeAndValidate(run); err != nil {
		return run.response, err
	}

	if req.Atomic && !req.DryRun {
		err := i.store.RunInTx(ctx, func(mutator store.ImportMutator) error {
			if err := i.applyTaskUpserts(ctx, run, mutator); err != nil {
				return err
			}
			if err := i.applyDependencies(ctx, run, mutator); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return run.response, err
		}
		run.response.AppliedChunks = 1
		return run.response, nil
	}

	if err := i.applyTaskUpserts(ctx, run, i.store); err != nil {
		return run.response, err
	}
	if err := i.applyDependencies(ctx, run, i.store); err != nil {
		return run.response, err
	}
	if !req.DryRun {
		run.response.AppliedChunks = 1
	}

	return run.response, nil
}

func (i *Importer) normalizeAndValidate(run *importRun) error {
	for idx, raw := range run.req.Tasks {
		rec, skip, err := normalizeImportRecord(raw)
		if err != nil {
			return badRequest(err)
		}
		if skip {
			run.actions[idx] = importActionError
			run.response.Errors++
			run.response.Messages = append(run.response.Messages, "skipping record with missing id or title")
			continue
		}

		run.normalized[idx] = rec
		run.importIDs[rec.ID] = true
	}
	return nil
}

func (i *Importer) applyTaskUpserts(ctx context.Context, run *importRun, mutator store.ImportMutator) error {
	for idx, rec := range run.normalized {
		if rec.ID == "" {
			continue
		}

		exists, err := i.taskExists(run, mutator, rec.ID)
		if err != nil {
			return err
		}

		if exists {
			switch run.dedupe {
			case "skip":
				run.actions[idx] = importActionSkipped
				run.response.Skipped++
				run.response.TaskIDs = append(run.response.TaskIDs, rec.ID)
				continue
			case "error":
				run.actions[idx] = importActionError
				run.response.Errors++
				run.response.Messages = append(run.response.Messages, fmt.Sprintf("duplicate id: %s", rec.ID))
				continue
			case "overwrite":
				run.actions[idx] = importActionUpdated
				if !run.req.DryRun {
					if err := i.overwriteTask(ctx, mutator, rec); err != nil {
						return err
					}
				}
				run.response.Updated++
				run.taskExistsCache[rec.ID] = true
				run.response.TaskIDs = append(run.response.TaskIDs, rec.ID)
			}
			continue
		}

		run.actions[idx] = importActionCreated
		if !run.req.DryRun {
			task := rec.Task
			if err := mutator.CreateTask(ctx, &task, rec.Labels, nil); err != nil {
				return err
			}
		}
		run.response.Created++
		run.taskExistsCache[rec.ID] = true
		run.response.TaskIDs = append(run.response.TaskIDs, rec.ID)
	}

	return nil
}

func (i *Importer) overwriteTask(ctx context.Context, mutator store.ImportMutator, rec api.TaskImportRecord) error {
	update := buildTaskUpdateFromImport(rec)
	if err := mutator.UpdateTask(ctx, rec.ID, update); err != nil {
		return err
	}
	if rec.Labels != nil {
		if err := mutator.ReplaceLabels(ctx, rec.ID, rec.Labels); err != nil {
			return err
		}
	}
	return nil
}

func (i *Importer) applyDependencies(ctx context.Context, run *importRun, mutator store.ImportMutator) error {
	if run.req.DryRun {
		return nil
	}

	for idx, rec := range run.normalized {
		action := run.actions[idx]
		if action != importActionCreated && action != importActionUpdated {
			continue
		}
		if rec.Deps == nil {
			continue
		}

		deps, skipRecord, err := i.resolvedDeps(run, mutator, rec)
		if err != nil {
			return err
		}
		if skipRecord {
			continue
		}

		if action == importActionUpdated {
			if err := mutator.RemoveDependencies(ctx, rec.ID); err != nil {
				return err
			}
		}

		for _, dep := range deps {
			if err := mutator.AddDependency(ctx, rec.ID, dep.ParentID, dep.Type); err != nil {
				return err
			}
		}
	}

	return nil
}

func (i *Importer) resolvedDeps(run *importRun, mutator store.ImportMutator, rec api.TaskImportRecord) ([]models.Dependency, bool, error) {
	if run.orphanHandling == "allow" {
		return rec.Deps, false, nil
	}

	deps := make([]models.Dependency, 0, len(rec.Deps))
	strictOrphans := make([]string, 0)

	for _, dep := range rec.Deps {
		inBatch := run.importIDs[dep.ParentID]
		exists, err := i.taskExists(run, mutator, dep.ParentID)
		if err != nil {
			return nil, false, err
		}
		if !exists && !inBatch {
			if run.orphanHandling == "strict" {
				strictOrphans = append(strictOrphans, dep.ParentID)
				continue
			}
			run.response.Messages = append(run.response.Messages, fmt.Sprintf("skipped orphan dep: %s -> %s", rec.ID, dep.ParentID))
			continue
		}
		deps = append(deps, dep)
	}

	if len(strictOrphans) > 0 {
		for _, parentID := range strictOrphans {
			run.response.Errors++
			run.response.Messages = append(run.response.Messages, fmt.Sprintf("strict orphan dep: %s -> %s (dependencies unchanged)", rec.ID, parentID))
		}
		return nil, true, nil
	}

	return deps, false, nil
}

func (i *Importer) taskExists(run *importRun, mutator store.ImportMutator, id string) (bool, error) {
	exists, ok := run.taskExistsCache[id]
	if ok {
		return exists, nil
	}

	exists, err := mutator.TaskExists(id)
	if err != nil {
		return false, err
	}
	run.taskExistsCache[id] = exists
	return exists, nil
}

func normalizeImportRecord(rec api.TaskImportRecord) (api.TaskImportRecord, bool, error) {
	rec.ID = strings.TrimSpace(rec.ID)
	rec.Title = strings.TrimSpace(rec.Title)
	if rec.ID == "" || rec.Title == "" {
		return rec, true, nil
	}
	if !validateID(rec.ID) {
		return rec, false, fmt.Errorf("invalid id: %s", rec.ID)
	}

	status, err := normalizeStatus(rec.Status)
	if err != nil {
		return rec, false, err
	}
	rec.Status = status

	taskType, err := normalizeType(rec.Type)
	if err != nil {
		return rec, false, err
	}
	rec.Type = taskType

	if !models.IsValidPriority(rec.Priority) {
		return rec, false, fmt.Errorf("priority must be between %d and %d", models.PriorityMin, models.PriorityMax)
	}

	rec.ParentID = strings.TrimSpace(rec.ParentID)
	if rec.ParentID != "" && !validateID(rec.ParentID) {
		return rec, false, fmt.Errorf("invalid parent_id")
	}

	if rec.Labels != nil {
		labels, err := normalizeLabels(rec.Labels)
		if err != nil {
			return rec, false, err
		}
		rec.Labels = labels
	}

	if rec.Deps != nil {
		deps := make([]models.Dependency, 0, len(rec.Deps))
		for _, dep := range rec.Deps {
			parentID := strings.TrimSpace(dep.ParentID)
			if parentID == "" || !validateID(parentID) {
				return rec, false, fmt.Errorf("invalid dependency parent_id")
			}
			depType := strings.TrimSpace(dep.Type)
			if depType == "" {
				depType = string(models.DependencyBlocks)
			}
			deps = append(deps, models.Dependency{ParentID: parentID, Type: depType})
		}
		rec.Deps = deps
	}

	now := time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = rec.CreatedAt
	}
	if rec.Status == string(models.StatusClosed) {
		if rec.ClosedAt == nil || rec.ClosedAt.IsZero() {
			closedAt := rec.UpdatedAt
			rec.ClosedAt = &closedAt
		}
	} else {
		rec.ClosedAt = nil
	}

	return rec, false, nil
}
