package main

import (
	"errors"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

func newCloseCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close <id> [<id>...]",
		Short: "Close tasks",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("id is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				resp, err := client.CloseTasks(cmd.Context(), api.TaskCloseRequest{IDs: args})
				if err != nil {
					return err
				}
				if *jsonOutput {
					return writeJSON(resp)
				}
				return writePlain("%v\n", args)
			})
		},
	}

	return cmd
}
