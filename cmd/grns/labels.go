package main

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

type labelMutationFunc func(context.Context, *api.Client, string, api.LabelsRequest) ([]string, error)

func runLabelMutation(cfg *config.Config, jsonOutput bool, ctx context.Context, args []string, mutate labelMutationFunc) error {
	label := args[len(args)-1]
	ids := args[:len(args)-1]
	return withClient(cfg, func(client *api.Client) error {
		var last []string
		for _, id := range ids {
			labels, err := mutate(ctx, client, id, api.LabelsRequest{Labels: []string{label}})
			if err != nil {
				return err
			}
			last = labels
		}
		return writeLabels(last, jsonOutput)
	})
}

func writeLabels(labels []string, jsonOutput bool) error {
	if jsonOutput {
		return writeJSON(labels)
	}
	return writePlain("%s\n", strings.Join(labels, ","))
}

func newLabelCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	labelCmd := &cobra.Command{
		Use:   "label",
		Short: "Manage labels",
	}

	labelCmd.AddCommand(
		newLabelAddCmd(cfg, jsonOutput),
		newLabelRemoveCmd(cfg, jsonOutput),
		newLabelListCmd(cfg, jsonOutput),
		newLabelListAllCmd(cfg, jsonOutput),
	)
	return labelCmd
}

func newLabelAddCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <id> [<id>...] <label>",
		Short: "Add a label",
		Args:  requireAtLeastArgs(2, "id(s) and label are required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLabelMutation(cfg, *jsonOutput, cmd.Context(), args,
				func(ctx context.Context, client *api.Client, id string, req api.LabelsRequest) ([]string, error) {
					return client.AddLabels(ctx, id, req)
				},
			)
		},
	}
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func newLabelRemoveCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <id> [<id>...] <label>",
		Short: "Remove a label",
		Args:  requireAtLeastArgs(2, "id(s) and label are required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLabelMutation(cfg, *jsonOutput, cmd.Context(), args,
				func(ctx context.Context, client *api.Client, id string, req api.LabelsRequest) ([]string, error) {
					return client.RemoveLabels(ctx, id, req)
				},
			)
		},
	}
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func newLabelListCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "list <id>",
		Short: "List labels for a task",
		Args:  requireExactlyArgs(1, "id is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				labels, err := client.ListLabels(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				return writeLabels(labels, *jsonOutput)
			})
		},
	}
}

func newLabelListAllCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "list-all",
		Short: "List all labels",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				labels, err := client.ListAllLabels(cmd.Context())
				if err != nil {
					return err
				}
				return writeLabels(labels, *jsonOutput)
			})
		},
	}
}
