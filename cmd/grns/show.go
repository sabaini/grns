package main

import (
	"errors"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

func newShowCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id> [<id>...]",
		Short: "Show task details",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("id is required")
			}
			return nil
		},
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

				responses := make([]api.TaskResponse, 0, len(args))
				for _, id := range args {
					resp, err := client.GetTask(cmd.Context(), id)
					if err != nil {
						return err
					}
					responses = append(responses, resp)
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
