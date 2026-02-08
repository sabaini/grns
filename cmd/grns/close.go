package main

import (
	"strings"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

type closeCmdOptions struct {
	commit string
	repo   string
}

func newCloseCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	opts := &closeCmdOptions{}
	cmd := &cobra.Command{
		Use:   "close <id> [<id>...]",
		Short: "Close tasks",
		Args:  requireAtLeastOneID,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				resp, err := client.CloseTasks(cmd.Context(), api.TaskCloseRequest{
					IDs:    args,
					Commit: strings.TrimSpace(opts.commit),
					Repo:   strings.TrimSpace(opts.repo),
				})
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

	cmd.Flags().StringVar(&opts.commit, "commit", "", "git commit hash to annotate closed tasks")
	cmd.Flags().StringVar(&opts.repo, "repo", "", "repository slug (host/owner/repo) for close annotation")
	return cmd
}
