package main

import (
	"net/url"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

type listCmdOptions struct {
	status           string
	priority         string
	priorityMin      string
	priorityMax      string
	taskType         string
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
}

func newListCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	opts := &listCmdOptions{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, cfg, opts, jsonOutput)
		},
	}

	bindListFlags(cmd, opts)
	return cmd
}

func runList(cmd *cobra.Command, cfg *config.Config, opts *listCmdOptions, jsonOutput *bool) error {
	return withClient(cfg, func(client *api.Client) error {
		query := buildListQueryValues(opts)
		resp, err := client.ListTasks(cmd.Context(), query)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return writeJSON(resp)
		}
		return writeTaskList(resp)
	})
}

func buildListQueryValues(opts *listCmdOptions) url.Values {
	query := url.Values{}
	setIfNotEmpty(query, "status", opts.status)
	setIfNotEmpty(query, "priority", opts.priority)
	setIfNotEmpty(query, "priority_min", opts.priorityMin)
	setIfNotEmpty(query, "priority_max", opts.priorityMax)
	setIfNotEmpty(query, "type", opts.taskType)
	setIfNotEmpty(query, "label", opts.label)
	setIfNotEmpty(query, "label_any", opts.labelAny)
	setIfNotEmpty(query, "spec", opts.spec)
	setIfNotEmpty(query, "parent_id", opts.parentID)
	setIfNotEmpty(query, "assignee", opts.assignee)
	if opts.noAssignee {
		query.Set("no_assignee", "true")
	}
	setIfNotEmpty(query, "id", opts.ids)
	setIfNotEmpty(query, "title_contains", opts.titleContains)
	setIfNotEmpty(query, "desc_contains", opts.descContains)
	setIfNotEmpty(query, "notes_contains", opts.notesContains)
	setIfNotEmpty(query, "created_after", opts.createdAfter)
	setIfNotEmpty(query, "created_before", opts.createdBefore)
	setIfNotEmpty(query, "updated_after", opts.updatedAfter)
	setIfNotEmpty(query, "updated_before", opts.updatedBefore)
	setIfNotEmpty(query, "closed_after", opts.closedAfter)
	setIfNotEmpty(query, "closed_before", opts.closedBefore)
	if opts.emptyDescription {
		query.Set("empty_description", "true")
	}
	if opts.noLabels {
		query.Set("no_labels", "true")
	}
	setIfNotEmpty(query, "search", opts.search)
	if opts.limit > 0 {
		query.Set("limit", intToString(opts.limit))
	}
	if opts.offset > 0 {
		query.Set("offset", intToString(opts.offset))
	}
	return query
}

func bindListFlags(cmd *cobra.Command, opts *listCmdOptions) {
	cmd.Flags().StringVar(&opts.status, "status", "", "status filter")
	cmd.Flags().StringVar(&opts.priority, "priority", "", "priority filter")
	cmd.Flags().StringVar(&opts.priorityMin, "priority-min", "", "priority min")
	cmd.Flags().StringVar(&opts.priorityMax, "priority-max", "", "priority max")
	cmd.Flags().StringVar(&opts.taskType, "type", "", "type filter")
	cmd.Flags().StringVar(&opts.label, "label", "", "label filter")
	cmd.Flags().StringVar(&opts.labelAny, "label-any", "", "label any filter")
	cmd.Flags().StringVar(&opts.spec, "spec", "", "spec regex")
	cmd.Flags().StringVar(&opts.parentID, "parent", "", "parent id")
	cmd.Flags().StringVar(&opts.assignee, "assignee", "", "assignee filter")
	cmd.Flags().BoolVar(&opts.noAssignee, "no-assignee", false, "unassigned tasks only")
	cmd.Flags().StringVar(&opts.ids, "id", "", "filter by ids (comma-separated)")
	cmd.Flags().StringVar(&opts.titleContains, "title-contains", "", "title contains text")
	cmd.Flags().StringVar(&opts.descContains, "desc-contains", "", "description contains text")
	cmd.Flags().StringVar(&opts.notesContains, "notes-contains", "", "notes contains text")
	cmd.Flags().StringVar(&opts.createdAfter, "created-after", "", "created after (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&opts.createdBefore, "created-before", "", "created before (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&opts.updatedAfter, "updated-after", "", "updated after (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&opts.updatedBefore, "updated-before", "", "updated before (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&opts.closedAfter, "closed-after", "", "closed after (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&opts.closedBefore, "closed-before", "", "closed before (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().BoolVar(&opts.emptyDescription, "empty-description", false, "tasks with no description")
	cmd.Flags().BoolVar(&opts.noLabels, "no-labels", false, "tasks with no labels")
	cmd.Flags().StringVar(&opts.search, "search", "", "full-text search query")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "limit results")
	cmd.Flags().IntVar(&opts.offset, "offset", 0, "offset results")
}
