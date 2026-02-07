package main

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

type updateCmdOptions struct {
	title              string
	status             string
	taskType           string
	priority           int
	description        string
	specID             string
	parentID           string
	assignee           string
	notes              string
	design             string
	acceptanceCriteria string
	sourceRepo         string
	customKV           []string
	customJSON         string
}

func newUpdateCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	opts := &updateCmdOptions{}
	cmd := &cobra.Command{
		Use:   "update <id> [<id>...]",
		Short: "Update tasks",
		Args:  requireAtLeastOneID,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd, cfg, opts, jsonOutput, args)
		},
	}

	bindUpdateFlags(cmd, opts)
	return cmd
}

func runUpdate(cmd *cobra.Command, cfg *config.Config, opts *updateCmdOptions, jsonOutput *bool, args []string) error {
	return withClient(cfg, func(client *api.Client) error {
		req, err := buildUpdateRequest(cmd, opts)
		if err != nil {
			return err
		}
		if !hasTaskUpdateFields(req) {
			return errors.New("no fields to update")
		}

		responses := make([]api.TaskResponse, 0, len(args))
		for _, id := range args {
			resp, err := client.UpdateTask(cmd.Context(), id, req)
			if err != nil {
				return err
			}
			responses = append(responses, resp)
		}
		if *jsonOutput {
			if len(responses) == 1 {
				return writeJSON(responses[0])
			}
			return writeJSON(responses)
		}
		return writePlain("%s\n", strings.Join(args, ","))
	})
}

func buildUpdateRequest(cmd *cobra.Command, opts *updateCmdOptions) (api.TaskUpdateRequest, error) {
	req := api.TaskUpdateRequest{}
	if cmd.Flags().Changed("title") {
		req.Title = &opts.title
	}
	if cmd.Flags().Changed("status") {
		req.Status = &opts.status
	}
	if cmd.Flags().Changed("type") {
		req.Type = &opts.taskType
	}
	if cmd.Flags().Changed("priority") {
		req.Priority = &opts.priority
	}
	if cmd.Flags().Changed("description") {
		req.Description = &opts.description
	}
	if cmd.Flags().Changed("spec-id") {
		req.SpecID = &opts.specID
	}
	if cmd.Flags().Changed("parent") {
		req.ParentID = &opts.parentID
	}
	if cmd.Flags().Changed("assignee") {
		req.Assignee = &opts.assignee
	}
	if cmd.Flags().Changed("notes") {
		req.Notes = &opts.notes
	}
	if cmd.Flags().Changed("design") {
		req.Design = &opts.design
	}
	if cmd.Flags().Changed("acceptance") {
		req.AcceptanceCriteria = &opts.acceptanceCriteria
	}
	if cmd.Flags().Changed("source-repo") {
		req.SourceRepo = &opts.sourceRepo
	}
	if len(opts.customKV) > 0 || opts.customJSON != "" {
		m, err := parseCustomFlags(opts.customKV, opts.customJSON)
		if err != nil {
			return api.TaskUpdateRequest{}, err
		}
		req.Custom = m
	}

	return req, nil
}

func hasTaskUpdateFields(req api.TaskUpdateRequest) bool {
	return req.Title != nil ||
		req.Status != nil ||
		req.Type != nil ||
		req.Priority != nil ||
		req.Description != nil ||
		req.SpecID != nil ||
		req.ParentID != nil ||
		req.Assignee != nil ||
		req.Notes != nil ||
		req.Design != nil ||
		req.AcceptanceCriteria != nil ||
		req.SourceRepo != nil ||
		req.Custom != nil
}

func bindUpdateFlags(cmd *cobra.Command, opts *updateCmdOptions) {
	cmd.Flags().StringVar(&opts.title, "title", "", "new title")
	cmd.Flags().StringVar(&opts.status, "status", "", "status")
	cmd.Flags().StringVarP(&opts.taskType, "type", "t", "", "type")
	cmd.Flags().IntVarP(&opts.priority, "priority", "p", 0, "priority")
	cmd.Flags().StringVarP(&opts.description, "description", "d", "", "description")
	cmd.Flags().StringVar(&opts.specID, "spec-id", "", "spec id")
	cmd.Flags().StringVar(&opts.parentID, "parent", "", "parent id")
	cmd.Flags().StringVar(&opts.assignee, "assignee", "", "assignee")
	cmd.Flags().StringVar(&opts.notes, "notes", "", "notes")
	cmd.Flags().StringVar(&opts.design, "design", "", "design")
	cmd.Flags().StringVar(&opts.acceptanceCriteria, "acceptance", "", "acceptance criteria")
	cmd.Flags().StringVar(&opts.sourceRepo, "source-repo", "", "source repository")
	cmd.Flags().StringSliceVar(&opts.customKV, "custom", nil, "custom field key=value (repeatable)")
	cmd.Flags().StringVar(&opts.customJSON, "custom-json", "", "custom fields as JSON object")
}
