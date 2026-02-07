package main

import (
	"context"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

func newReopenCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reopen <id> [<id>...]",
		Short: "Reopen tasks",
		Args:  requireAtLeastOneID,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIDsMutation(cfg, *jsonOutput, cmd.Context(), args,
				func(ctx context.Context, client *api.Client, ids []string) (any, error) {
					return client.ReopenTasks(ctx, api.TaskReopenRequest{IDs: ids})
				},
			)
		},
	}

	return cmd
}
