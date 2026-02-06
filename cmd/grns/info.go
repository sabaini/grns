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
				resp.DBPath = cfg.DBPath

				if *jsonOutput {
					return writeJSON(resp)
				}

				_ = writePlain("db_path: %s\n", resp.DBPath)
				_ = writePlain("project_prefix: %s\n", resp.ProjectPrefix)
				_ = writePlain("schema_version: %d\n", resp.SchemaVersion)
				_ = writePlain("total_tasks: %d\n", resp.TotalTasks)

				statuses := make([]string, 0, len(resp.TaskCounts))
				for status := range resp.TaskCounts {
					statuses = append(statuses, status)
				}
				sort.Strings(statuses)
				for _, status := range statuses {
					_ = writePlain("  %s: %d\n", status, resp.TaskCounts[status])
				}
				return nil
			})
		},
	}
	return cmd
}
