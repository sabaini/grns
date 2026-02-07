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
