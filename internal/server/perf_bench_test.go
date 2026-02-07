package server

import (
	"context"
	"testing"
)

func BenchmarkTaskServiceListFiltered(b *testing.B) {
	service := newPerfTaskService(b, 3000)
	ctx := context.Background()
	filter := taskListFilter{
		Statuses:  []string{"open", "in_progress"},
		LabelsAny: []string{"auth"},
		Limit:     75,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		responses, err := service.List(ctx, filter)
		if err != nil {
			b.Fatalf("list filtered: %v", err)
		}
		if len(responses) == 0 {
			b.Fatal("list filtered returned no results")
		}
	}
}

func BenchmarkTaskServiceListSearch(b *testing.B) {
	service := newPerfTaskService(b, 3000)
	ctx := context.Background()
	filter := taskListFilter{
		SearchQuery: "auth",
		Limit:       75,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		responses, err := service.List(ctx, filter)
		if err != nil {
			b.Fatalf("list search: %v", err)
		}
		if len(responses) == 0 {
			b.Fatal("list search returned no results")
		}
	}
}

func BenchmarkTaskServiceBatchCreate(b *testing.B) {
	service := newPerfTaskService(b, 200)
	ctx := context.Background()
	const batchSize = 40
	idBase := 500000

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		requests := perfBatchCreateRequests(idBase+i*batchSize, batchSize)
		if _, err := service.BatchCreate(ctx, requests); err != nil {
			b.Fatalf("batch create: %v", err)
		}
	}
}

func BenchmarkTaskServiceImportOverwrite(b *testing.B) {
	service := newPerfTaskService(b, 500)
	ctx := context.Background()
	const importStart = 100
	const importCount = 120
	request := perfImportOverwriteRequest(importStart, importCount, true)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := service.Import(ctx, request)
		if err != nil {
			b.Fatalf("import overwrite: %v", err)
		}
		if resp.Updated != importCount {
			b.Fatalf("unexpected import updated count: got %d want %d", resp.Updated, importCount)
		}
	}
}
