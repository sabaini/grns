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

func newCreateCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	var (
		id                 string
		issueType          string
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
	)

	cmd := &cobra.Command{
		Use:   "create <title>",
		Short: "Create a new task",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				if filePath != "" {
					return runCreateFromFile(client, filePath, jsonOutput)
				}
				if len(args) == 0 {
					return errors.New("title is required")
				}

				req := api.TaskCreateRequest{
					ID:    id,
					Title: strings.Join(args, " "),
				}
				if issueType != "" {
					req.Type = &issueType
				}
				if cmd.Flags().Changed("priority") {
					req.Priority = &priority
				}
				if description != "" {
					req.Description = &description
				}
				if specID != "" {
					req.SpecID = &specID
				}
				if parentID != "" {
					req.ParentID = &parentID
				}
				if assignee != "" {
					req.Assignee = &assignee
				}
				if notes != "" {
					req.Notes = &notes
				}
				if design != "" {
					req.Design = &design
				}
				if acceptanceCriteria != "" {
					req.AcceptanceCriteria = &acceptanceCriteria
				}
				if sourceRepo != "" {
					req.SourceRepo = &sourceRepo
				}
				if len(labels) > 0 {
					req.Labels = labels
				}
				if deps != "" {
					depList, err := parseDeps(deps)
					if err != nil {
						return err
					}
					req.Deps = depList
				}
				if len(customKV) > 0 || customJSON != "" {
					m, err := parseCustomFlags(customKV, customJSON)
					if err != nil {
						return err
					}
					req.Custom = m
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
		},
	}

	cmd.Flags().StringVar(&id, "id", "", "explicit task id")
	cmd.Flags().StringVarP(&issueType, "type", "t", "", "task type")
	cmd.Flags().IntVarP(&priority, "priority", "p", 0, "task priority")
	cmd.Flags().StringVarP(&description, "description", "d", "", "task description")
	cmd.Flags().StringVar(&specID, "spec-id", "", "spec id")
	cmd.Flags().StringVar(&parentID, "parent", "", "parent task id")
	cmd.Flags().StringVar(&assignee, "assignee", "", "assignee")
	cmd.Flags().StringVar(&notes, "notes", "", "notes")
	cmd.Flags().StringVar(&design, "design", "", "design")
	cmd.Flags().StringVar(&acceptanceCriteria, "acceptance", "", "acceptance criteria")
	cmd.Flags().StringVar(&sourceRepo, "source-repo", "", "source repository")
	cmd.Flags().StringSliceVarP(&labels, "label", "l", nil, "labels")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "labels")
	cmd.Flags().StringVar(&deps, "deps", "", "dependencies")
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "markdown file for batch create")
	cmd.Flags().StringSliceVar(&customKV, "custom", nil, "custom field key=value (repeatable)")
	cmd.Flags().StringVar(&customJSON, "custom-json", "", "custom fields as JSON object")

	return cmd
}

func runCreateFromFile(client *api.Client, filePath string, jsonOutput *bool) error {
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

	resp, err := client.BatchCreate(context.Background(), requests)
	if err != nil {
		return err
	}

	if *jsonOutput {
		return writeJSON(resp)
	}
	for _, task := range resp {
		_ = writePlain("%s\n", task.ID)
	}
	return nil
}
