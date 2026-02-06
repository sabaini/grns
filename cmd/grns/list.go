package main

import (
	"net/url"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

func newListCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	var (
		status      string
		priority    string
		priorityMin string
		priorityMax string
		issueType   string
		label       string
		labelAny    string
		spec        string
		parentID    string
		limit       int
		offset      int
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
	cmd.Flags().IntVar(&limit, "limit", 0, "limit results")
	cmd.Flags().IntVar(&offset, "offset", 0, "offset results")

	return cmd
}
