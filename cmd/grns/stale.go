package main

import (
	"net/url"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

func newStaleCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	var (
		days   int
		status string
		limit  int
	)

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "List stale tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				query := url.Values{}
				if days > 0 {
					query.Set("days", intToString(days))
				}
				setIfNotEmpty(query, "status", status)
				if limit > 0 {
					query.Set("limit", intToString(limit))
				}
				resp, err := client.Stale(cmd.Context(), query)
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

	cmd.Flags().IntVar(&days, "days", 30, "days since update")
	cmd.Flags().StringVar(&status, "status", "", "status filter")
	cmd.Flags().IntVar(&limit, "limit", 0, "limit results")
	return cmd
}
