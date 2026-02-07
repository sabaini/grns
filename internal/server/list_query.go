package server

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"grns/internal/models"
	"grns/internal/store"
)

func parseListFilter(r *http.Request) (store.ListFilter, error) {
	limit, err := queryInt(r, "limit")
	if err != nil {
		return store.ListFilter{}, err
	}
	offset, err := queryInt(r, "offset")
	if err != nil {
		return store.ListFilter{}, err
	}

	filter := store.ListFilter{
		Statuses:  splitCSV(r.URL.Query().Get("status")),
		Types:     splitCSV(r.URL.Query().Get("type")),
		ParentID:  strings.TrimSpace(r.URL.Query().Get("parent_id")),
		Labels:    splitCSV(r.URL.Query().Get("label")),
		LabelsAny: splitCSV(r.URL.Query().Get("label_any")),
		Limit:     limit,
		Offset:    offset,
	}

	if filter.ParentID != "" && !validateID(filter.ParentID) {
		return store.ListFilter{}, fmt.Errorf("invalid parent_id")
	}

	if len(filter.Statuses) > 0 {
		statuses := make([]string, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			value, err := normalizeStatus(status)
			if err != nil {
				return store.ListFilter{}, err
			}
			statuses = append(statuses, value)
		}
		filter.Statuses = statuses
	}

	if len(filter.Types) > 0 {
		types := make([]string, 0, len(filter.Types))
		for _, taskType := range filter.Types {
			value, err := normalizeType(taskType)
			if err != nil {
				return store.ListFilter{}, err
			}
			types = append(types, value)
		}
		filter.Types = types
	}

	if len(filter.Labels) > 0 {
		labels, err := normalizeLabels(filter.Labels)
		if err != nil {
			return store.ListFilter{}, err
		}
		filter.Labels = labels
	}
	if len(filter.LabelsAny) > 0 {
		labels, err := normalizeLabels(filter.LabelsAny)
		if err != nil {
			return store.ListFilter{}, err
		}
		filter.LabelsAny = labels
	}

	priority, err := parsePriorityQuery(r.URL.Query().Get("priority"), "priority")
	if err != nil {
		return store.ListFilter{}, err
	}
	filter.Priority = priority

	priorityMin, err := parsePriorityQuery(r.URL.Query().Get("priority_min"), "priority_min")
	if err != nil {
		return store.ListFilter{}, err
	}
	filter.PriorityMin = priorityMin

	priorityMax, err := parsePriorityQuery(r.URL.Query().Get("priority_max"), "priority_max")
	if err != nil {
		return store.ListFilter{}, err
	}
	filter.PriorityMax = priorityMax

	if filter.PriorityMin != nil && filter.PriorityMax != nil {
		if *filter.PriorityMin > *filter.PriorityMax {
			return store.ListFilter{}, fmt.Errorf("priority_min cannot be greater than priority_max")
		}
	}

	if assignee := strings.TrimSpace(r.URL.Query().Get("assignee")); assignee != "" {
		filter.Assignee = assignee
	}
	if r.URL.Query().Get("no_assignee") == "true" {
		filter.NoAssignee = true
	}
	if ids := splitCSV(r.URL.Query().Get("id")); len(ids) > 0 {
		filter.IDs = ids
	}
	if value := strings.TrimSpace(r.URL.Query().Get("title_contains")); value != "" {
		filter.TitleContains = value
	}
	if value := strings.TrimSpace(r.URL.Query().Get("desc_contains")); value != "" {
		filter.DescContains = value
	}
	if value := strings.TrimSpace(r.URL.Query().Get("notes_contains")); value != "" {
		filter.NotesContains = value
	}

	createdAfter, err := parseTimeFilter(r, "created_after")
	if err != nil {
		return store.ListFilter{}, fmt.Errorf("invalid created_after: %w", err)
	}
	filter.CreatedAfter = createdAfter

	createdBefore, err := parseTimeFilter(r, "created_before")
	if err != nil {
		return store.ListFilter{}, fmt.Errorf("invalid created_before: %w", err)
	}
	filter.CreatedBefore = createdBefore

	updatedAfter, err := parseTimeFilter(r, "updated_after")
	if err != nil {
		return store.ListFilter{}, fmt.Errorf("invalid updated_after: %w", err)
	}
	filter.UpdatedAfter = updatedAfter

	updatedBefore, err := parseTimeFilter(r, "updated_before")
	if err != nil {
		return store.ListFilter{}, fmt.Errorf("invalid updated_before: %w", err)
	}
	filter.UpdatedBefore = updatedBefore

	closedAfter, err := parseTimeFilter(r, "closed_after")
	if err != nil {
		return store.ListFilter{}, fmt.Errorf("invalid closed_after: %w", err)
	}
	filter.ClosedAfter = closedAfter

	closedBefore, err := parseTimeFilter(r, "closed_before")
	if err != nil {
		return store.ListFilter{}, fmt.Errorf("invalid closed_before: %w", err)
	}
	filter.ClosedBefore = closedBefore

	if r.URL.Query().Get("empty_description") == "true" {
		filter.EmptyDescription = true
	}
	if r.URL.Query().Get("no_labels") == "true" {
		filter.NoLabels = true
	}
	if search := strings.TrimSpace(r.URL.Query().Get("search")); search != "" {
		filter.SearchQuery = search
	}

	spec := strings.TrimSpace(r.URL.Query().Get("spec"))
	if spec != "" {
		pattern := "(?i)" + spec
		if _, err := regexp.Compile(pattern); err != nil {
			return store.ListFilter{}, fmt.Errorf("invalid spec regex")
		}
		filter.SpecRegex = pattern
	}

	return filter, nil
}

func parsePriorityQuery(raw, key string) (*int, error) {
	if raw == "" {
		return nil, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s", key)
	}
	if !models.IsValidPriority(value) {
		return nil, fmt.Errorf("%s must be between %d and %d", key, models.PriorityMin, models.PriorityMax)
	}

	return &value, nil
}

func parseTimeFilter(r *http.Request, key string) (*time.Time, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return nil, nil
	}

	parsed, err := parseFlexibleTime(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
