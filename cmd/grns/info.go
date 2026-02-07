package main

import (
	"sort"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

func newInfoCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show database and project info",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				resp, err := client.GetInfo(cmd.Context())
				if err != nil {
					return err
				}

				output := struct {
					DBPath        string         `json:"db_path"`
					ProjectPrefix string         `json:"project_prefix"`
					SchemaVersion int            `json:"schema_version"`
					TaskCounts    map[string]int `json:"task_counts"`
					TotalTasks    int            `json:"total_tasks"`
				}{
					DBPath:        cfg.DBPath,
					ProjectPrefix: resp.ProjectPrefix,
					SchemaVersion: resp.SchemaVersion,
					TaskCounts:    resp.TaskCounts,
					TotalTasks:    resp.TotalTasks,
				}

				if *jsonOutput {
					return writeJSON(output)
				}

				if err := writePlain("db_path: %s\n", output.DBPath); err != nil {
					return err
				}
				if err := writePlain("project_prefix: %s\n", output.ProjectPrefix); err != nil {
					return err
				}
				if err := writePlain("schema_version: %d\n", output.SchemaVersion); err != nil {
					return err
				}
				if err := writePlain("total_tasks: %d\n", output.TotalTasks); err != nil {
					return err
				}

				statuses := make([]string, 0, len(output.TaskCounts))
				for status := range output.TaskCounts {
					statuses = append(statuses, status)
				}
				sort.Strings(statuses)
				for _, status := range statuses {
					if err := writePlain("  %s: %d\n", status, output.TaskCounts[status]); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
	return cmd
}
