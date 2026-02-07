package models

import (
	"fmt"
	"strings"
)

// TaskStatus defines allowed lifecycle states for tasks.
type TaskStatus string

const (
	StatusOpen       TaskStatus = "open"
	StatusInProgress TaskStatus = "in_progress"
	StatusBlocked    TaskStatus = "blocked"
	StatusDeferred   TaskStatus = "deferred"
	StatusClosed     TaskStatus = "closed"
	StatusTombstone  TaskStatus = "tombstone"
	StatusPinned     TaskStatus = "pinned"
)

// TaskType defines allowed task categories.
type TaskType string

const (
	TypeBug     TaskType = "bug"
	TypeFeature TaskType = "feature"
	TypeTask    TaskType = "task"
	TypeEpic    TaskType = "epic"
	TypeChore   TaskType = "chore"
)

// DependencyType defines supported dependency edge kinds.
type DependencyType string

const (
	DependencyBlocks DependencyType = "blocks"
)

const (
	PriorityMin     = 0
	PriorityMax     = 4
	DefaultPriority = 2

	DependencyTreeMaxDepth = 50
)

var validTaskStatuses = map[TaskStatus]struct{}{
	StatusOpen:       {},
	StatusInProgress: {},
	StatusBlocked:    {},
	StatusDeferred:   {},
	StatusClosed:     {},
	StatusTombstone:  {},
	StatusPinned:     {},
}

var validTaskTypes = map[TaskType]struct{}{
	TypeBug:     {},
	TypeFeature: {},
	TypeTask:    {},
	TypeEpic:    {},
	TypeChore:   {},
}

var readyTaskStatuses = []TaskStatus{
	StatusOpen,
	StatusInProgress,
	StatusBlocked,
	StatusDeferred,
	StatusPinned,
}

var staleDefaultExcludedStatuses = []TaskStatus{
	StatusClosed,
	StatusTombstone,
}

func IsValidTaskStatus(status TaskStatus) bool {
	_, ok := validTaskStatuses[status]
	return ok
}

func IsValidTaskType(taskType TaskType) bool {
	_, ok := validTaskTypes[taskType]
	return ok
}

func ParseTaskStatus(raw string) (TaskStatus, error) {
	value := TaskStatus(strings.ToLower(strings.TrimSpace(raw)))
	if value == "" {
		return "", fmt.Errorf("status is required")
	}
	if !IsValidTaskStatus(value) {
		return "", fmt.Errorf("invalid status: %s", value)
	}
	return value, nil
}

func ParseTaskType(raw string) (TaskType, error) {
	value := TaskType(strings.ToLower(strings.TrimSpace(raw)))
	if value == "" {
		return "", fmt.Errorf("type is required")
	}
	if !IsValidTaskType(value) {
		return "", fmt.Errorf("invalid type: %s", value)
	}
	return value, nil
}

func ReadyTaskStatusStrings() []string {
	return statusStrings(readyTaskStatuses)
}

func StaleDefaultExcludedStatusStrings() []string {
	return statusStrings(staleDefaultExcludedStatuses)
}

func IsValidPriority(value int) bool {
	return value >= PriorityMin && value <= PriorityMax
}

func statusStrings(values []TaskStatus) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}
