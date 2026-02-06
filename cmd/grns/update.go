package main

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

func newUpdateCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	var (
		title              string
		status             string
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
		customKV           []string
		customJSON         string
	)

	cmd := &cobra.Command{
		Use:   "update <id> [<id>...]",
		Short: "Update tasks",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("id is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				req := api.TaskUpdateRequest{}
				if cmd.Flags().Changed("title") {
					req.Title = &title
				}
				if cmd.Flags().Changed("status") {
					req.Status = &status
				}
				if cmd.Flags().Changed("type") {
					req.Type = &issueType
				}
				if cmd.Flags().Changed("priority") {
					req.Priority = &priority
				}
				if cmd.Flags().Changed("description") {
					req.Description = &description
				}
				if cmd.Flags().Changed("spec-id") {
					req.SpecID = &specID
				}
				if cmd.Flags().Changed("parent") {
					req.ParentID = &parentID
				}
				if cmd.Flags().Changed("assignee") {
					req.Assignee = &assignee
				}
				if cmd.Flags().Changed("notes") {
					req.Notes = &notes
				}
				if cmd.Flags().Changed("design") {
					req.Design = &design
				}
				if cmd.Flags().Changed("acceptance") {
					req.AcceptanceCriteria = &acceptanceCriteria
				}
				if cmd.Flags().Changed("source-repo") {
					req.SourceRepo = &sourceRepo
				}
				if len(customKV) > 0 || customJSON != "" {
					m, err := parseCustomFlags(customKV, customJSON)
					if err != nil {
						return err
					}
					req.Custom = m
				}

				if req.Title == nil && req.Status == nil && req.Type == nil && req.Priority == nil && req.Description == nil && req.SpecID == nil && req.ParentID == nil && req.Assignee == nil && req.Notes == nil && req.Design == nil && req.AcceptanceCriteria == nil && req.SourceRepo == nil && req.Custom == nil {
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
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&status, "status", "", "status")
	cmd.Flags().StringVarP(&issueType, "type", "t", "", "type")
	cmd.Flags().IntVarP(&priority, "priority", "p", 0, "priority")
	cmd.Flags().StringVarP(&description, "description", "d", "", "description")
	cmd.Flags().StringVar(&specID, "spec-id", "", "spec id")
	cmd.Flags().StringVar(&parentID, "parent", "", "parent id")
	cmd.Flags().StringVar(&assignee, "assignee", "", "assignee")
	cmd.Flags().StringVar(&notes, "notes", "", "notes")
	cmd.Flags().StringVar(&design, "design", "", "design")
	cmd.Flags().StringVar(&acceptanceCriteria, "acceptance", "", "acceptance criteria")
	cmd.Flags().StringVar(&sourceRepo, "source-repo", "", "source repository")
	cmd.Flags().StringSliceVar(&customKV, "custom", nil, "custom field key=value (repeatable)")
	cmd.Flags().StringVar(&customJSON, "custom-json", "", "custom fields as JSON object")

	return cmd
}
