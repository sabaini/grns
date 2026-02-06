package main

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

func newLabelCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	labelCmd := &cobra.Command{
		Use:   "label",
		Short: "Manage labels",
	}

	addCmd := &cobra.Command{
		Use:   "add <id> [<id>...] <label>",
		Short: "Add a label",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return errors.New("id(s) and label are required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			label := args[len(args)-1]
			ids := args[:len(args)-1]
			return withClient(cfg, func(client *api.Client) error {
				var last []string
				for _, id := range ids {
					labels, err := client.AddLabels(cmd.Context(), id, api.LabelsRequest{Labels: []string{label}})
					if err != nil {
						return err
					}
					last = labels
				}
				if *jsonOutput {
					return writeJSON(last)
				}
				return writePlain("%s\n", strings.Join(last, ","))
			})
		},
	}

	removeCmd := &cobra.Command{
		Use:   "remove <id> [<id>...] <label>",
		Short: "Remove a label",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return errors.New("id(s) and label are required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			label := args[len(args)-1]
			ids := args[:len(args)-1]
			return withClient(cfg, func(client *api.Client) error {
				var last []string
				for _, id := range ids {
					labels, err := client.RemoveLabels(cmd.Context(), id, api.LabelsRequest{Labels: []string{label}})
					if err != nil {
						return err
					}
					last = labels
				}
				if *jsonOutput {
					return writeJSON(last)
				}
				return writePlain("%s\n", strings.Join(last, ","))
			})
		},
	}

	listCmd := &cobra.Command{
		Use:   "list <id>",
		Short: "List labels for a task",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("id is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				labels, err := client.ListLabels(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				if *jsonOutput {
					return writeJSON(labels)
				}
				return writePlain("%s\n", strings.Join(labels, ","))
			})
		},
	}

	listAllCmd := &cobra.Command{
		Use:   "list-all",
		Short: "List all labels",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				labels, err := client.ListAllLabels(cmd.Context())
				if err != nil {
					return err
				}
				if *jsonOutput {
					return writeJSON(labels)
				}
				return writePlain("%s\n", strings.Join(labels, ","))
			})
		},
	}

	labelCmd.AddCommand(addCmd, removeCmd, listCmd, listAllCmd)
	return labelCmd
}
