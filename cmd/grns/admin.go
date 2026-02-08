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
	cmd.AddCommand(newAdminGCBlobsCmd(cfg, jsonOutput))
	return cmd
}

func newAdminCleanupCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	var (
		olderThan int
		dryRun    bool
		force     bool
		project   string
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
					Project:       project,
				}
				resp, err := client.AdminCleanup(cmd.Context(), req, force)
				if err != nil {
					return err
				}

				if *jsonOutput {
					return writeJSON(resp)
				}

				if resp.DryRun {
					if err := writePlain("dry run: %d closed tasks would be removed\n", resp.Count); err != nil {
						return err
					}
				} else {
					if err := writePlain("removed %d closed tasks\n", resp.Count); err != nil {
						return err
					}
				}
				for _, id := range resp.TaskIDs {
					if err := writePlain("  %s\n", id); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}

	cmd.Flags().IntVar(&olderThan, "older-than", 0, "remove tasks closed/updated more than N days ago (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be removed without deleting")
	cmd.Flags().BoolVar(&force, "force", false, "actually delete tasks (required for non-dry-run)")
	cmd.Flags().StringVar(&project, "project", "", "optional project scope for cleanup (e.g. gr)")

	return cmd
}

func newAdminGCBlobsCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	var (
		dryRun    bool
		apply     bool
		batchSize int
	)

	cmd := &cobra.Command{
		Use:   "gc-blobs",
		Short: "Garbage-collect unreferenced managed blobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !apply && !dryRun {
				dryRun = true
			}

			return withClient(cfg, func(client *api.Client) error {
				req := api.BlobGCRequest{DryRun: !apply, BatchSize: batchSize}
				resp, err := client.AdminGCBlobs(cmd.Context(), req, apply)
				if err != nil {
					return err
				}
				if *jsonOutput {
					return writeJSON(resp)
				}
				mode := "dry run"
				if !resp.DryRun {
					mode = "applied"
				}
				return writePlain("%s: candidates=%d deleted=%d failed=%d reclaimed_bytes=%d\n", mode, resp.CandidateCount, resp.DeletedCount, resp.FailedCount, resp.ReclaimedBytes)
			})
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be reclaimed without deleting")
	cmd.Flags().BoolVar(&apply, "apply", false, "delete unreferenced blobs")
	cmd.Flags().IntVar(&batchSize, "batch-size", 0, "apply-mode batch size (default: server attachment gc batch size)")
	return cmd
}
