package main

import (
	"net/url"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

func newListCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	var (
		status           string
		priority         string
		priorityMin      string
		priorityMax      string
		issueType        string
		label            string
		labelAny         string
		spec             string
		parentID         string
		assignee         string
		noAssignee       bool
		ids              string
		titleContains    string
		descContains     string
		notesContains    string
		createdAfter     string
		createdBefore    string
		updatedAfter     string
		updatedBefore    string
		closedAfter      string
		closedBefore     string
		emptyDescription bool
		noLabels         bool
		search           string
		limit            int
		offset           int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				query := url.Values{}
				setIfNotEmpty(query, "status", status)
				setIfNotEmpty(query, "priority", priority)
				setIfNotEmpty(query, "priority_min", priorityMin)
				setIfNotEmpty(query, "priority_max", priorityMax)
				setIfNotEmpty(query, "type", issueType)
				setIfNotEmpty(query, "label", label)
				setIfNotEmpty(query, "label_any", labelAny)
				setIfNotEmpty(query, "spec", spec)
				setIfNotEmpty(query, "parent_id", parentID)
				setIfNotEmpty(query, "assignee", assignee)
				if noAssignee {
					query.Set("no_assignee", "true")
				}
				setIfNotEmpty(query, "id", ids)
				setIfNotEmpty(query, "title_contains", titleContains)
				setIfNotEmpty(query, "desc_contains", descContains)
				setIfNotEmpty(query, "notes_contains", notesContains)
				setIfNotEmpty(query, "created_after", createdAfter)
				setIfNotEmpty(query, "created_before", createdBefore)
				setIfNotEmpty(query, "updated_after", updatedAfter)
				setIfNotEmpty(query, "updated_before", updatedBefore)
				setIfNotEmpty(query, "closed_after", closedAfter)
				setIfNotEmpty(query, "closed_before", closedBefore)
				if emptyDescription {
					query.Set("empty_description", "true")
				}
				if noLabels {
					query.Set("no_labels", "true")
				}
				setIfNotEmpty(query, "search", search)
				if limit > 0 {
					query.Set("limit", intToString(limit))
				}
				if offset > 0 {
					query.Set("offset", intToString(offset))
				}

				resp, err := client.ListTasks(cmd.Context(), query)
				if err != nil {
					return err
				}
				if *jsonOutput {
					return writeJSON(resp)
				}
				return writeTaskList(resp)
			})
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "status filter")
	cmd.Flags().StringVar(&priority, "priority", "", "priority filter")
	cmd.Flags().StringVar(&priorityMin, "priority-min", "", "priority min")
	cmd.Flags().StringVar(&priorityMax, "priority-max", "", "priority max")
	cmd.Flags().StringVar(&issueType, "type", "", "type filter")
	cmd.Flags().StringVar(&label, "label", "", "label filter")
	cmd.Flags().StringVar(&labelAny, "label-any", "", "label any filter")
	cmd.Flags().StringVar(&spec, "spec", "", "spec regex")
	cmd.Flags().StringVar(&parentID, "parent", "", "parent id")
	cmd.Flags().StringVar(&assignee, "assignee", "", "assignee filter")
	cmd.Flags().BoolVar(&noAssignee, "no-assignee", false, "unassigned tasks only")
	cmd.Flags().StringVar(&ids, "id", "", "filter by ids (comma-separated)")
	cmd.Flags().StringVar(&titleContains, "title-contains", "", "title contains text")
	cmd.Flags().StringVar(&descContains, "desc-contains", "", "description contains text")
	cmd.Flags().StringVar(&notesContains, "notes-contains", "", "notes contains text")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "created after (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&createdBefore, "created-before", "", "created before (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&updatedAfter, "updated-after", "", "updated after (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&updatedBefore, "updated-before", "", "updated before (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&closedAfter, "closed-after", "", "closed after (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&closedBefore, "closed-before", "", "closed before (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().BoolVar(&emptyDescription, "empty-description", false, "tasks with no description")
	cmd.Flags().BoolVar(&noLabels, "no-labels", false, "tasks with no labels")
	cmd.Flags().StringVar(&search, "search", "", "full-text search query")
	cmd.Flags().IntVar(&limit, "limit", 0, "limit results")
	cmd.Flags().IntVar(&offset, "offset", 0, "offset results")

	return cmd
}
