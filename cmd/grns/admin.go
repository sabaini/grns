package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

func newAdminCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Administrative commands",
	}

	cmd.AddCommand(newAdminCleanupCmd(cfg, jsonOutput))
	return cmd
}

func newAdminCleanupCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	var (
		olderThan int
		dryRun    bool
		force     bool
	)

	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Remove old closed tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if olderThan <= 0 {
				return fmt.Errorf("--older-than must be > 0")
			}
			if !force && !dryRun {
				dryRun = true
			}

			return withClient(cfg, func(client *api.Client) error {
				req := api.CleanupRequest{
					OlderThanDays: olderThan,
					DryRun:        dryRun,
				}
				resp, err := client.AdminCleanup(cmd.Context(), req, force)
				if err != nil {
					return err
				}

				if *jsonOutput {
					return writeJSON(resp)
				}

				if resp.DryRun {
					_ = writePlain("dry run: %d closed tasks would be removed\n", resp.Count)
				} else {
					_ = writePlain("removed %d closed tasks\n", resp.Count)
				}
				for _, id := range resp.TaskIDs {
					_ = writePlain("  %s\n", id)
				}
				return nil
			})
		},
	}

	cmd.Flags().IntVar(&olderThan, "older-than", 0, "remove tasks closed/updated more than N days ago (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be removed without deleting")
	cmd.Flags().BoolVar(&force, "force", false, "actually delete tasks (required for non-dry-run)")

	return cmd
}
