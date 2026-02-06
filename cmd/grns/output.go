package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"grns/internal/api"
	"grns/internal/format"
)

var outputFormatter format.Formatter = format.JSONFormatter{}

func writeJSON(payload any) error {
	return outputFormatter.Write(os.Stdout, payload)
}

func writePlain(format string, args ...any) error {
	_, err := fmt.Fprintf(os.Stdout, format, args...)
	return err
}

func writeTaskList(tasks []api.TaskResponse) error {
	for _, task := range tasks {
		if err := writePlain("%s\n", formatTaskLine(task)); err != nil {
			return err
		}
	}
	return nil
}

func writeTaskDetail(task api.TaskResponse) error {
	lines := []string{
		fmt.Sprintf("id: %s", task.ID),
		fmt.Sprintf("title: %s", task.Title),
		fmt.Sprintf("status: %s", task.Status),
		fmt.Sprintf("type: %s", task.Type),
		fmt.Sprintf("priority: %d", task.Priority),
		fmt.Sprintf("created_at: %s", formatTime(task.CreatedAt)),
		fmt.Sprintf("updated_at: %s", formatTime(task.UpdatedAt)),
	}

	if task.SpecID != "" {
		lines = append(lines, fmt.Sprintf("spec_id: %s", task.SpecID))
	}
	if task.ParentID != "" {
		lines = append(lines, fmt.Sprintf("parent_id: %s", task.ParentID))
	}
	if task.Assignee != "" {
		lines = append(lines, fmt.Sprintf("assignee: %s", task.Assignee))
	}
	if task.SourceRepo != "" {
		lines = append(lines, fmt.Sprintf("source_repo: %s", task.SourceRepo))
	}
	if task.Description != "" {
		lines = append(lines, fmt.Sprintf("description: %s", task.Description))
	}
	if task.Notes != "" {
		lines = append(lines, fmt.Sprintf("notes: %s", task.Notes))
	}
	if task.Design != "" {
		lines = append(lines, fmt.Sprintf("design: %s", task.Design))
	}
	if task.AcceptanceCriteria != "" {
		lines = append(lines, fmt.Sprintf("acceptance_criteria: %s", task.AcceptanceCriteria))
	}
	if task.ClosedAt != nil {
		lines = append(lines, fmt.Sprintf("closed_at: %s", formatTime(*task.ClosedAt)))
	}

	if len(task.Labels) > 0 {
		lines = append(lines, fmt.Sprintf("labels: %s", strings.Join(task.Labels, ", ")))
	}
	if len(task.Deps) > 0 {
		lines = append(lines, "deps:")
		for _, dep := range task.Deps {
			lines = append(lines, fmt.Sprintf("  - %s: %s", dep.Type, dep.ParentID))
		}
	}

	return writePlain("%s\n", strings.Join(lines, "\n"))
}

func formatTaskLine(task api.TaskResponse) string {
	return fmt.Sprintf("○ %s [P%d] [%s] - %s", task.ID, task.Priority, task.Type, task.Title)
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
