package store

import (
	"fmt"
	"strings"
)

type listQueryBuilder struct {
	filter ListFilter
	query  string
	args   []any
	where  []string
}

func buildListQuery(filter ListFilter) (string, []any) {
	builder := &listQueryBuilder{filter: filter}
	builder.buildSelect()
	builder.buildWhere()
	builder.buildOrder()
	builder.buildPagination()
	return builder.query, builder.args
}

func (b *listQueryBuilder) buildSelect() {
	b.query = "SELECT " + taskColumns + " FROM tasks"
	if b.filter.SearchQuery == "" {
		return
	}
	b.query = "SELECT " + qualifiedTaskColumns + " FROM tasks JOIN tasks_fts ON tasks.id = tasks_fts.task_id AND tasks_fts MATCH ?"
	b.args = append(b.args, b.filter.SearchQuery)
}

func (b *listQueryBuilder) buildWhere() {
	b.appendProject()
	b.appendStatuses()
	b.appendTypes()
	b.appendPriority()
	b.appendParentID()
	b.appendLabels()
	b.appendAssignee()
	b.appendIDs()
	b.appendContainsFilters()
	b.appendTimeFilters()
	b.appendEmptyDescription()
	b.appendNoLabels()

	if len(b.where) == 0 {
		return
	}
	b.query += " WHERE " + strings.Join(b.where, " AND ")
}

func (b *listQueryBuilder) buildOrder() {
	if b.filter.SearchQuery != "" {
		b.query += " ORDER BY tasks_fts.rank"
		return
	}
	b.query += " ORDER BY updated_at DESC"
}

func (b *listQueryBuilder) buildPagination() {
	if b.filter.SpecRegex != "" {
		return
	}

	hasLimit := false
	if b.filter.Limit > 0 {
		b.query += " LIMIT ?"
		b.args = append(b.args, b.filter.Limit)
		hasLimit = true
	}
	if b.filter.Offset > 0 {
		if !hasLimit {
			b.query += " LIMIT -1"
		}
		b.query += " OFFSET ?"
		b.args = append(b.args, b.filter.Offset)
	}
}

func (b *listQueryBuilder) appendProject() {
	if b.filter.Project == "" {
		return
	}
	b.where = append(b.where, "tasks.project_id = ?")
	b.args = append(b.args, normalizeProject(b.filter.Project))
}

func (b *listQueryBuilder) appendStatuses() {
	if len(b.filter.Statuses) == 0 {
		return
	}
	b.where = append(b.where, fmt.Sprintf("status IN (%s)", placeholders(len(b.filter.Statuses))))
	for _, status := range b.filter.Statuses {
		b.args = append(b.args, status)
	}
}

func (b *listQueryBuilder) appendTypes() {
	if len(b.filter.Types) == 0 {
		return
	}
	b.where = append(b.where, fmt.Sprintf("type IN (%s)", placeholders(len(b.filter.Types))))
	for _, taskType := range b.filter.Types {
		b.args = append(b.args, taskType)
	}
}

func (b *listQueryBuilder) appendPriority() {
	if b.filter.Priority != nil {
		b.where = append(b.where, "priority = ?")
		b.args = append(b.args, *b.filter.Priority)
	}
	if b.filter.PriorityMin != nil {
		b.where = append(b.where, "priority >= ?")
		b.args = append(b.args, *b.filter.PriorityMin)
	}
	if b.filter.PriorityMax != nil {
		b.where = append(b.where, "priority <= ?")
		b.args = append(b.args, *b.filter.PriorityMax)
	}
}

func (b *listQueryBuilder) appendParentID() {
	if b.filter.ParentID == "" {
		return
	}
	b.where = append(b.where, "parent_id = ?")
	b.args = append(b.args, b.filter.ParentID)
}

func (b *listQueryBuilder) appendLabels() {
	if len(b.filter.Labels) > 0 {
		b.where = append(b.where, fmt.Sprintf("id IN (SELECT task_id FROM task_labels WHERE label IN (%s) GROUP BY task_id HAVING COUNT(DISTINCT label) = %d)", placeholders(len(b.filter.Labels)), len(b.filter.Labels)))
		for _, label := range b.filter.Labels {
			b.args = append(b.args, label)
		}
	}
	if len(b.filter.LabelsAny) > 0 {
		b.where = append(b.where, fmt.Sprintf("id IN (SELECT task_id FROM task_labels WHERE label IN (%s))", placeholders(len(b.filter.LabelsAny))))
		for _, label := range b.filter.LabelsAny {
			b.args = append(b.args, label)
		}
	}
}

func (b *listQueryBuilder) appendAssignee() {
	if b.filter.Assignee != "" {
		b.where = append(b.where, "assignee = ?")
		b.args = append(b.args, b.filter.Assignee)
	}
	if b.filter.NoAssignee {
		b.where = append(b.where, "(assignee IS NULL OR assignee = '')")
	}
}

func (b *listQueryBuilder) appendIDs() {
	if len(b.filter.IDs) == 0 {
		return
	}
	b.where = append(b.where, fmt.Sprintf("id IN (%s)", placeholders(len(b.filter.IDs))))
	for _, id := range b.filter.IDs {
		b.args = append(b.args, id)
	}
}

func (b *listQueryBuilder) appendContainsFilters() {
	if b.filter.TitleContains != "" {
		b.where = append(b.where, "tasks.title LIKE '%' || ? || '%'")
		b.args = append(b.args, b.filter.TitleContains)
	}
	if b.filter.DescContains != "" {
		b.where = append(b.where, "tasks.description LIKE '%' || ? || '%'")
		b.args = append(b.args, b.filter.DescContains)
	}
	if b.filter.NotesContains != "" {
		b.where = append(b.where, "tasks.notes LIKE '%' || ? || '%'")
		b.args = append(b.args, b.filter.NotesContains)
	}
}

func (b *listQueryBuilder) appendTimeFilters() {
	if b.filter.CreatedAfter != nil {
		b.where = append(b.where, "created_at > ?")
		b.args = append(b.args, dbFormatTime(*b.filter.CreatedAfter))
	}
	if b.filter.CreatedBefore != nil {
		b.where = append(b.where, "created_at < ?")
		b.args = append(b.args, dbFormatTime(*b.filter.CreatedBefore))
	}
	if b.filter.UpdatedAfter != nil {
		b.where = append(b.where, "updated_at > ?")
		b.args = append(b.args, dbFormatTime(*b.filter.UpdatedAfter))
	}
	if b.filter.UpdatedBefore != nil {
		b.where = append(b.where, "updated_at < ?")
		b.args = append(b.args, dbFormatTime(*b.filter.UpdatedBefore))
	}
	if b.filter.ClosedAfter != nil {
		b.where = append(b.where, "closed_at > ?")
		b.args = append(b.args, dbFormatTime(*b.filter.ClosedAfter))
	}
	if b.filter.ClosedBefore != nil {
		b.where = append(b.where, "closed_at < ?")
		b.args = append(b.args, dbFormatTime(*b.filter.ClosedBefore))
	}
}

func (b *listQueryBuilder) appendEmptyDescription() {
	if !b.filter.EmptyDescription {
		return
	}
	b.where = append(b.where, "(tasks.description IS NULL OR tasks.description = '')")
}

func (b *listQueryBuilder) appendNoLabels() {
	if !b.filter.NoLabels {
		return
	}
	b.where = append(b.where, "id NOT IN (SELECT task_id FROM task_labels)")
}
