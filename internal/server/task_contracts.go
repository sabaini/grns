package server

import (
	"time"

	"grns/internal/store"
)

// taskListFilter is the service-layer query DTO.
type taskListFilter struct {
	Statuses         []string
	Types            []string
	Priority         *int
	PriorityMin      *int
	PriorityMax      *int
	ParentID         string
	Labels           []string
	LabelsAny        []string
	SpecRegex        string
	Assignee         string
	NoAssignee       bool
	IDs              []string
	TitleContains    string
	DescContains     string
	NotesContains    string
	CreatedAfter     *time.Time
	CreatedBefore    *time.Time
	UpdatedAfter     *time.Time
	UpdatedBefore    *time.Time
	ClosedAfter      *time.Time
	ClosedBefore     *time.Time
	EmptyDescription bool
	NoLabels         bool
	SearchQuery      string
	Limit            int
	Offset           int
}

func (f taskListFilter) toStoreListFilter() store.ListFilter {
	return store.ListFilter{
		Statuses:         f.Statuses,
		Types:            f.Types,
		Priority:         f.Priority,
		PriorityMin:      f.PriorityMin,
		PriorityMax:      f.PriorityMax,
		ParentID:         f.ParentID,
		Labels:           f.Labels,
		LabelsAny:        f.LabelsAny,
		SpecRegex:        f.SpecRegex,
		Assignee:         f.Assignee,
		NoAssignee:       f.NoAssignee,
		IDs:              f.IDs,
		TitleContains:    f.TitleContains,
		DescContains:     f.DescContains,
		NotesContains:    f.NotesContains,
		CreatedAfter:     f.CreatedAfter,
		CreatedBefore:    f.CreatedBefore,
		UpdatedAfter:     f.UpdatedAfter,
		UpdatedBefore:    f.UpdatedBefore,
		ClosedAfter:      f.ClosedAfter,
		ClosedBefore:     f.ClosedBefore,
		EmptyDescription: f.EmptyDescription,
		NoLabels:         f.NoLabels,
		SearchQuery:      f.SearchQuery,
		Limit:            f.Limit,
		Offset:           f.Offset,
	}
}

// taskUpdatePatch is the service-layer task mutation DTO.
type taskUpdatePatch struct {
	Title              *string
	Status             *string
	Type               *string
	Priority           *int
	Description        *string
	SpecID             *string
	ParentID           *string
	Assignee           *string
	Notes              *string
	Design             *string
	AcceptanceCriteria *string
	SourceRepo         *string
	ClosedAt           *time.Time
	Custom             *map[string]any
	UpdatedAt          time.Time
}

func (p taskUpdatePatch) toStoreTaskUpdate() store.TaskUpdate {
	return store.TaskUpdate{
		Title:              p.Title,
		Status:             p.Status,
		Type:               p.Type,
		Priority:           p.Priority,
		Description:        p.Description,
		SpecID:             p.SpecID,
		ParentID:           p.ParentID,
		Assignee:           p.Assignee,
		Notes:              p.Notes,
		Design:             p.Design,
		AcceptanceCriteria: p.AcceptanceCriteria,
		SourceRepo:         p.SourceRepo,
		ClosedAt:           p.ClosedAt,
		Custom:             p.Custom,
		UpdatedAt:          p.UpdatedAt,
	}
}
