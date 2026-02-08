package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"grns/internal/api"
	"grns/internal/models"
)

func TestTaskServiceCreate_ValidationMatrix(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, svc *TaskService)
		req        api.TaskCreateRequest
		wantStatus int
		wantCode   int
	}{
		{
			name:       "missing title",
			req:        api.TaskCreateRequest{},
			wantStatus: 400,
			wantCode:   ErrCodeMissingRequired,
		},
		{
			name:       "invalid explicit id format",
			req:        api.TaskCreateRequest{ID: "bad-id", Title: "x"},
			wantStatus: 400,
			wantCode:   ErrCodeInvalidID,
		},
		{
			name:       "wrong prefix id",
			req:        api.TaskCreateRequest{ID: "zz-ab12", Title: "x"},
			wantStatus: 400,
			wantCode:   ErrCodeInvalidID,
		},
		{
			name:       "invalid parent id",
			req:        api.TaskCreateRequest{Title: "x", ParentID: strPtr("not-an-id")},
			wantStatus: 400,
			wantCode:   ErrCodeInvalidParentID,
		},
		{
			name:       "invalid label",
			req:        api.TaskCreateRequest{Title: "x", Labels: []string{"has space"}},
			wantStatus: 400,
			wantCode:   ErrCodeInvalidLabel,
		},
		{
			name: "missing dependency parent",
			req: api.TaskCreateRequest{
				Title: "x",
				Deps:  []models.Dependency{{ParentID: "gr-zz99", Type: "blocks"}},
			},
			wantStatus: 400,
			wantCode:   ErrCodeInvalidDependency,
		},
		{
			name:       "invalid priority",
			req:        api.TaskCreateRequest{Title: "x", Priority: intPtrRef(9)},
			wantStatus: 400,
			wantCode:   ErrCodeInvalidPriority,
		},
		{
			name: "duplicate explicit id",
			setup: func(t *testing.T, svc *TaskService) {
				t.Helper()
				_, err := svc.Create(context.Background(), api.TaskCreateRequest{ID: "gr-du11", Title: "first"})
				if err != nil {
					t.Fatalf("seed duplicate id: %v", err)
				}
			},
			req:        api.TaskCreateRequest{ID: "gr-du11", Title: "dup"},
			wantStatus: 409,
			wantCode:   ErrCodeTaskIDExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newTaskServiceForTest(t)
			if tt.setup != nil {
				tt.setup(t, svc)
			}

			_, err := svc.Create(context.Background(), tt.req)
			if err == nil {
				t.Fatal("expected validation/conflict error")
			}
			assertAPIErrorStatusAndCode(t, err, tt.wantStatus, tt.wantCode)
		})
	}
}

func TestTaskServiceGetMany_MixedMissingIDsReturnsNotFound(t *testing.T) {
	svc, st := newTaskServiceForTest(t)
	ctx := context.Background()
	now := time.Now().UTC()

	mustCreateTask(t, st, &models.Task{ID: "gr-gm11", Title: "first", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}, nil, nil)
	mustCreateTask(t, st, &models.Task{ID: "gr-gm22", Title: "second", Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}, nil, nil)

	t.Run("mixed existing and missing ids returns not found", func(t *testing.T) {
		_, err := svc.GetMany(ctx, []string{"gr-gm11", "gr-zz99"})
		if err == nil {
			t.Fatal("expected not found error")
		}
		assertAPIErrorStatusAndCode(t, err, 404, ErrCodeTaskNotFound)
	})

	t.Run("duplicate ids preserve full request order", func(t *testing.T) {
		responses, err := svc.GetMany(ctx, []string{"gr-gm22", "gr-gm22", "gr-gm11"})
		if err != nil {
			t.Fatalf("get many with duplicate ids: %v", err)
		}
		if len(responses) != 3 {
			t.Fatalf("expected 3 responses, got %d", len(responses))
		}
		if responses[0].ID != "gr-gm22" || responses[1].ID != "gr-gm22" || responses[2].ID != "gr-gm11" {
			t.Fatalf("expected order [gr-gm22 gr-gm22 gr-gm11], got [%s %s %s]", responses[0].ID, responses[1].ID, responses[2].ID)
		}
	})
}

func TestTaskServiceBatchCreate_DependencyValidation(t *testing.T) {
	svc, _ := newTaskServiceForTest(t)
	ctx := context.Background()

	t.Run("allows forward dependency within batch", func(t *testing.T) {
		responses, err := svc.BatchCreate(ctx, []api.TaskCreateRequest{
			{ID: "gr-ch11", Title: "child", Deps: []models.Dependency{{ParentID: "gr-pa11", Type: "blocks"}}},
			{ID: "gr-pa11", Title: "parent"},
		})
		if err != nil {
			t.Fatalf("batch create with forward dependency: %v", err)
		}
		if len(responses) != 2 {
			t.Fatalf("expected 2 created tasks, got %d", len(responses))
		}
	})

	t.Run("missing dependency parent returns invalid dependency", func(t *testing.T) {
		_, err := svc.BatchCreate(ctx, []api.TaskCreateRequest{
			{ID: "gr-mc11", Title: "child", Deps: []models.Dependency{{ParentID: "gr-xx99", Type: "blocks"}}},
		})
		if err == nil {
			t.Fatal("expected invalid dependency error")
		}
		assertAPIErrorStatusAndCode(t, err, 400, ErrCodeInvalidDependency)
	})
}

func assertAPIErrorStatusAndCode(t *testing.T, err error, wantStatus, wantCode int) {
	t.Helper()
	if got := httpStatusFromError(err); got != wantStatus {
		t.Fatalf("expected HTTP %d, got %d (%v)", wantStatus, got, err)
	}
	var apiErr apiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected apiError, got %T", err)
	}
	if apiErr.errCode != wantCode {
		t.Fatalf("expected error_code %d, got %d", wantCode, apiErr.errCode)
	}
}

func intPtrRef(v int) *int { return &v }
