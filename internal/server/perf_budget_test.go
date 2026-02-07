package server

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPerformanceBudgets(t *testing.T) {
	if strings.TrimSpace(os.Getenv("GRNS_PERF_ENFORCE")) != "1" {
		t.Skip("set GRNS_PERF_ENFORCE=1 to run performance budget checks")
	}

	t.Run("list_filtered", func(t *testing.T) {
		service := newPerfTaskService(t, 3500)
		ctx := context.Background()
		filter := taskListFilter{Statuses: []string{"open", "in_progress"}, LabelsAny: []string{"auth"}, Limit: 100}
		rounds := envInt("GRNS_PERF_GO_LIST_FILTERED_ROUNDS", 160)
		maxPerOp := envDuration("GRNS_PERF_GO_LIST_FILTERED_MAX_PER_OP", 40*time.Millisecond)

		elapsed, total := runListRounds(t, service, ctx, filter, rounds)
		assertBudget(t, "list_filtered", elapsed, total, maxPerOp)
	})

	t.Run("list_search", func(t *testing.T) {
		service := newPerfTaskService(t, 3500)
		ctx := context.Background()
		filter := taskListFilter{SearchQuery: "auth", Limit: 100}
		rounds := envInt("GRNS_PERF_GO_LIST_SEARCH_ROUNDS", 120)
		maxPerOp := envDuration("GRNS_PERF_GO_LIST_SEARCH_MAX_PER_OP", 45*time.Millisecond)

		elapsed, total := runListRounds(t, service, ctx, filter, rounds)
		assertBudget(t, "list_search", elapsed, total, maxPerOp)
	})

	t.Run("batch_create", func(t *testing.T) {
		service := newPerfTaskService(t, 200)
		ctx := context.Background()
		rounds := envInt("GRNS_PERF_GO_BATCH_ROUNDS", 24)
		batchSize := envInt("GRNS_PERF_GO_BATCH_SIZE", 45)
		maxPerOp := envDuration("GRNS_PERF_GO_BATCH_MAX_PER_OP", 110*time.Millisecond)

		idBase := 700000
		started := time.Now()
		for i := 0; i < rounds; i++ {
			requests := perfBatchCreateRequests(idBase+i*batchSize, batchSize)
			if _, err := service.BatchCreate(ctx, requests); err != nil {
				t.Fatalf("batch create round %d: %v", i, err)
			}
		}
		elapsed := time.Since(started)
		assertBudget(t, "batch_create", elapsed, rounds, maxPerOp)
	})

	t.Run("import_overwrite", func(t *testing.T) {
		service := newPerfTaskService(t, 650)
		ctx := context.Background()
		rounds := envInt("GRNS_PERF_GO_IMPORT_ROUNDS", 20)
		importCount := envInt("GRNS_PERF_GO_IMPORT_COUNT", 120)
		maxPerOp := envDuration("GRNS_PERF_GO_IMPORT_MAX_PER_OP", 170*time.Millisecond)
		request := perfImportOverwriteRequest(100, importCount, true)

		started := time.Now()
		for i := 0; i < rounds; i++ {
			resp, err := service.Import(ctx, request)
			if err != nil {
				t.Fatalf("import round %d: %v", i, err)
			}
			if resp.Updated != importCount {
				t.Fatalf("import round %d updated mismatch: got %d want %d", i, resp.Updated, importCount)
			}
		}
		elapsed := time.Since(started)
		assertBudget(t, "import_overwrite", elapsed, rounds, maxPerOp)
	})
}

func runListRounds(t *testing.T, service *TaskService, ctx context.Context, filter taskListFilter, rounds int) (time.Duration, int) {
	t.Helper()

	started := time.Now()
	total := 0
	for i := 0; i < rounds; i++ {
		responses, err := service.List(ctx, filter)
		if err != nil {
			t.Fatalf("list round %d: %v", i, err)
		}
		total += len(responses)
	}
	if total == 0 {
		t.Fatal("list returned zero results across all rounds")
	}
	return time.Since(started), rounds
}

func assertBudget(t *testing.T, name string, elapsed time.Duration, ops int, maxPerOp time.Duration) {
	t.Helper()
	if ops <= 0 {
		t.Fatalf("%s: invalid op count %d", name, ops)
	}
	perOp := elapsed / time.Duration(ops)
	t.Logf("%s baseline: total=%s ops=%d per_op=%s budget=%s", name, elapsed, ops, perOp, maxPerOp)
	if perOp > maxPerOp {
		t.Fatalf("%s regression: per_op=%s exceeds budget=%s", name, perOp, maxPerOp)
	}
}

func envInt(key string, def int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return def
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return def
	}
	return parsed
}

func envDuration(key string, def time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return def
	}
	parsed, err := time.ParseDuration(value)
	if err == nil && parsed > 0 {
		return parsed
	}
	if millis, err := strconv.Atoi(value); err == nil && millis > 0 {
		return time.Duration(millis) * time.Millisecond
	}
	fmt.Fprintf(os.Stderr, "warning: invalid duration for %s=%q, using default %s\n", key, value, def)
	return def
}
