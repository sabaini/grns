package server

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"grns/internal/api"
	"grns/internal/models"
	"grns/internal/store"
)

const benchProjectPrefix = "gr"

func newPerfTaskService(tb testing.TB, seedCount int) *TaskService {
	tb.Helper()

	dbPath := filepath.Join(tb.TempDir(), "perf.db")
	st, err := store.Open(dbPath)
	if err != nil {
		tb.Fatalf("open perf store: %v", err)
	}
	tb.Cleanup(func() {
		_ = st.Close()
	})

	if seedCount > 0 {
		if err := seedPerfTasks(context.Background(), st, 0, seedCount); err != nil {
			tb.Fatalf("seed perf tasks: %v", err)
		}
	}

	return NewTaskService(st, benchProjectPrefix)
}

func seedPerfTasks(ctx context.Context, st *store.Store, start, count int) error {
	if count <= 0 {
		return nil
	}

	now := time.Now().UTC()
	batch := make([]store.TaskCreateInput, 0, count)
	for i := 0; i < count; i++ {
		idx := start + i
		taskID := perfTaskID(idx)
		status := []string{string(models.StatusOpen), string(models.StatusInProgress), string(models.StatusBlocked)}[idx%3]
		taskType := []string{string(models.TypeTask), string(models.TypeFeature), string(models.TypeBug)}[idx%3]
		title := fmt.Sprintf("Task %d worker", idx)
		description := fmt.Sprintf("Background processing ticket %d", idx)
		labels := []string{"infra"}
		if idx%5 == 0 {
			title = fmt.Sprintf("Task %d auth flow", idx)
			description = fmt.Sprintf("Authentication token refresh %d", idx)
			labels = []string{"auth", "backend"}
		}
		if idx%7 == 0 {
			labels = append(labels, "urgent")
		}

		batch = append(batch, store.TaskCreateInput{
			Task: &models.Task{
				ID:          taskID,
				Title:       title,
				Status:      status,
				Type:        taskType,
				Priority:    idx % 4,
				Description: description,
				SpecID:      fmt.Sprintf("SPEC-%03d", idx%100),
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			Labels: labels,
		})
	}
	return st.CreateTasks(ctx, batch)
}

func perfBatchCreateRequests(start, count int) []api.TaskCreateRequest {
	if count <= 0 {
		return nil
	}
	requests := make([]api.TaskCreateRequest, 0, count)
	for i := 0; i < count; i++ {
		idx := start + i
		requests = append(requests, api.TaskCreateRequest{
			ID:    perfTaskID(idx),
			Title: fmt.Sprintf("Batch task %d", idx),
			Type:  strPtr(string(models.TypeTask)),
			Labels: []string{
				"batch",
			},
		})
	}
	return requests
}

func perfImportOverwriteRequest(start, count int, atomic bool) api.ImportRequest {
	tasks := make([]api.TaskImportRecord, 0, count)
	now := time.Now().UTC()
	for i := 0; i < count; i++ {
		idx := start + i
		tasks = append(tasks, api.TaskImportRecord{
			Task: models.Task{
				ID:          perfTaskID(idx),
				Title:       fmt.Sprintf("Import overwrite %d", idx),
				Status:      string(models.StatusInProgress),
				Type:        string(models.TypeTask),
				Priority:    idx % 4,
				Description: fmt.Sprintf("Import overwrite description %d", idx),
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			Labels: []string{"imported", "perf"},
			Deps:   []models.Dependency{},
		})
	}
	return api.ImportRequest{
		Tasks:          tasks,
		Dedupe:         "overwrite",
		OrphanHandling: "allow",
		Atomic:         atomic,
	}
}

func perfTaskID(n int) string {
	const maxIDs = 36 * 36 * 36 * 36
	value := n % maxIDs
	if value < 0 {
		value += maxIDs
	}
	suffix := strings.ToLower(strconv.FormatInt(int64(value), 36))
	if len(suffix) < 4 {
		suffix = strings.Repeat("0", 4-len(suffix)) + suffix
	}
	return benchProjectPrefix + "-" + suffix
}

func strPtr(v string) *string {
	return &v
}
