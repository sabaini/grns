package models

import "testing"

func TestParseTaskStatus(t *testing.T) {
	got, err := ParseTaskStatus(" OPEN ")
	if err != nil {
		t.Fatalf("parse status: %v", err)
	}
	if got != StatusOpen {
		t.Fatalf("expected %q, got %q", StatusOpen, got)
	}

	if _, err := ParseTaskStatus("invalid"); err == nil {
		t.Fatal("expected invalid status error")
	}
}

func TestParseTaskType(t *testing.T) {
	got, err := ParseTaskType(" BUG ")
	if err != nil {
		t.Fatalf("parse type: %v", err)
	}
	if got != TypeBug {
		t.Fatalf("expected %q, got %q", TypeBug, got)
	}

	if _, err := ParseTaskType("invalid"); err == nil {
		t.Fatal("expected invalid type error")
	}
}

func TestIsValidPriority(t *testing.T) {
	if !IsValidPriority(DefaultPriority) {
		t.Fatalf("expected default priority %d to be valid", DefaultPriority)
	}
	if IsValidPriority(PriorityMin - 1) {
		t.Fatalf("expected %d to be invalid", PriorityMin-1)
	}
	if IsValidPriority(PriorityMax + 1) {
		t.Fatalf("expected %d to be invalid", PriorityMax+1)
	}
}

func TestOperationalStatusSets(t *testing.T) {
	ready := ReadyTaskStatusStrings()
	if len(ready) == 0 {
		t.Fatal("ready statuses must not be empty")
	}
	if ready[0] != string(StatusOpen) {
		t.Fatalf("expected ready statuses to include %q first, got %v", StatusOpen, ready)
	}

	excluded := StaleDefaultExcludedStatusStrings()
	if len(excluded) == 0 {
		t.Fatal("stale excluded statuses must not be empty")
	}
	foundClosed := false
	for _, status := range excluded {
		if status == string(StatusClosed) {
			foundClosed = true
			break
		}
	}
	if !foundClosed {
		t.Fatalf("expected stale excluded statuses to include %q, got %v", StatusClosed, excluded)
	}

	if DependencyTreeMaxDepth <= 0 {
		t.Fatalf("expected positive dependency tree max depth, got %d", DependencyTreeMaxDepth)
	}
}
