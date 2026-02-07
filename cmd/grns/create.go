package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

type createCmdOptions struct {
	id                 string
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
	labels             []string
	deps               string
	filePath           string
	customKV           []string
	customJSON         string
}

func newCreateCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	opts := &createCmdOptions{}
	cmd := &cobra.Command{
		Use:   "create <title>",
		Short: "Create a new task",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd, cfg, opts, jsonOutput, args)
		},
	}

	bindCreateFlags(cmd, opts)
	return cmd
}

func runCreate(cmd *cobra.Command, cfg *config.Config, opts *createCmdOptions, jsonOutput *bool, args []string) error {
	return withClient(cfg, func(client *api.Client) error {
		if opts.filePath != "" {
			return runCreateFromFile(cmd.Context(), client, opts.filePath, jsonOutput)
		}

		req, err := buildCreateRequest(cmd, opts, args)
		if err != nil {
			return err
		}

		resp, err := client.CreateTask(cmd.Context(), req)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return writeJSON(resp)
		}
		return writePlain("%s\n", resp.ID)
	})
}

func buildCreateRequest(cmd *cobra.Command, opts *createCmdOptions, args []string) (api.TaskCreateRequest, error) {
	if len(args) == 0 {
		return api.TaskCreateRequest{}, errors.New("title is required")
	}

	req := api.TaskCreateRequest{
		ID:    opts.id,
		Title: strings.Join(args, " "),
	}
	if opts.taskType != "" {
		req.Type = &opts.taskType
	}
	if cmd.Flags().Changed("priority") {
		req.Priority = &opts.priority
	}
	if opts.description != "" {
		req.Description = &opts.description
	}
	if opts.specID != "" {
		req.SpecID = &opts.specID
	}
	if opts.parentID != "" {
		req.ParentID = &opts.parentID
	}
	if opts.assignee != "" {
		req.Assignee = &opts.assignee
	}
	if opts.notes != "" {
		req.Notes = &opts.notes
	}
	if opts.design != "" {
		req.Design = &opts.design
	}
	if opts.acceptanceCriteria != "" {
		req.AcceptanceCriteria = &opts.acceptanceCriteria
	}
	if opts.sourceRepo != "" {
		req.SourceRepo = &opts.sourceRepo
	}
	if len(opts.labels) > 0 {
		req.Labels = opts.labels
	}
	if opts.deps != "" {
		depList, err := parseDeps(opts.deps)
		if err != nil {
			return api.TaskCreateRequest{}, err
		}
		req.Deps = depList
	}
	if len(opts.customKV) > 0 || opts.customJSON != "" {
		m, err := parseCustomFlags(opts.customKV, opts.customJSON)
		if err != nil {
			return api.TaskCreateRequest{}, err
		}
		req.Custom = m
	}

	return req, nil
}

func bindCreateFlags(cmd *cobra.Command, opts *createCmdOptions) {
	cmd.Flags().StringVar(&opts.id, "id", "", "explicit task id")
	cmd.Flags().StringVarP(&opts.taskType, "type", "t", "", "task type")
	cmd.Flags().IntVarP(&opts.priority, "priority", "p", 0, "task priority")
	cmd.Flags().StringVarP(&opts.description, "description", "d", "", "task description")
	cmd.Flags().StringVar(&opts.specID, "spec-id", "", "spec id")
	cmd.Flags().StringVar(&opts.parentID, "parent", "", "parent task id")
	cmd.Flags().StringVar(&opts.assignee, "assignee", "", "assignee")
	cmd.Flags().StringVar(&opts.notes, "notes", "", "notes")
	cmd.Flags().StringVar(&opts.design, "design", "", "design")
	cmd.Flags().StringVar(&opts.acceptanceCriteria, "acceptance", "", "acceptance criteria")
	cmd.Flags().StringVar(&opts.sourceRepo, "source-repo", "", "source repository")
	cmd.Flags().StringSliceVarP(&opts.labels, "label", "l", nil, "labels")
	cmd.Flags().StringSliceVar(&opts.labels, "labels", nil, "labels")
	cmd.Flags().StringVar(&opts.deps, "deps", "", "dependencies")
	cmd.Flags().StringVarP(&opts.filePath, "file", "f", "", "markdown file for batch create")
	cmd.Flags().StringSliceVar(&opts.customKV, "custom", nil, "custom field key=value (repeatable)")
	cmd.Flags().StringVar(&opts.customJSON, "custom-json", "", "custom fields as JSON object")
}

func runCreateFromFile(ctx context.Context, client *api.Client, filePath string, jsonOutput *bool) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	frontMatter, items, err := parseMarkdown(string(data))
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("no list items found in %s", filePath)
	}

	defaults, err := frontMatterToRequest(frontMatter)
	if err != nil {
		return err
	}
	requests := make([]api.TaskCreateRequest, 0, len(items))
	for _, item := range items {
		req := defaults
		req.Title = item
		requests = append(requests, req)
	}

	resp, err := client.BatchCreate(ctx, requests)
	if err != nil {
		return err
	}

	if *jsonOutput {
		return writeJSON(resp)
	}
	for _, task := range resp {
		if err := writePlain("%s\n", task.ID); err != nil {
			return err
		}
	}
	return nil
}
