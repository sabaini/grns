package main

import (
	"net/url"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

func newReadyCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "ready",
		Short: "List ready tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				query := url.Values{}
				if limit > 0 {
					query.Set("limit", intToString(limit))
				}
				resp, err := client.Ready(cmd.Context(), query)
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

	cmd.Flags().IntVar(&limit, "limit", 0, "limit results")
	return cmd
}
