package main

import (
	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

func newShowCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id> [<id>...]",
		Short: "Show task details",
		Args:  requireAtLeastOneID,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				if len(args) == 1 {
					resp, err := client.GetTask(cmd.Context(), args[0])
					if err != nil {
						return err
					}
					if *jsonOutput {
						return writeJSON(resp)
					}
					return writeTaskDetail(resp)
				}

				responses, err := client.GetTasks(cmd.Context(), args)
				if err != nil {
					return err
				}
				if *jsonOutput {
					return writeJSON(responses)
				}
				return writeTaskList(responses)
			})
		},
	}

	return cmd
}
