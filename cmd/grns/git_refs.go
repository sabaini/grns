package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
	"grns/internal/models"
)

type gitAddOptions struct {
	relation       string
	objectType     string
	objectValue    string
	repo           string
	resolvedCommit string
	note           string
}

func newGitCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{Use: "git", Short: "Manage task git references"}
	cmd.AddCommand(
		newGitAddCmd(cfg, jsonOutput),
		newGitListCmd(cfg, jsonOutput),
		newGitRemoveCmd(cfg, jsonOutput),
	)
	return cmd
}

func newGitAddCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	opts := &gitAddOptions{}
	cmd := &cobra.Command{
		Use:   "add <task-id>",
		Short: "Add a git reference to a task",
		Args:  requireExactlyArgs(1, "task id is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(opts.relation) == "" {
				return fmt.Errorf("--relation is required")
			}
			if strings.TrimSpace(opts.objectType) == "" {
				return fmt.Errorf("--type is required")
			}
			if strings.TrimSpace(opts.objectValue) == "" {
				return fmt.Errorf("--value is required")
			}

			req := api.TaskGitRefCreateRequest{
				Repo:           opts.repo,
				Relation:       opts.relation,
				ObjectType:     opts.objectType,
				ObjectValue:    opts.objectValue,
				ResolvedCommit: opts.resolvedCommit,
				Note:           opts.note,
			}

			return withClient(cfg, func(client *api.Client) error {
				ref, err := client.CreateTaskGitRef(cmd.Context(), args[0], req)
				if err != nil {
					return err
				}
				if *jsonOutput {
					return writeJSON(ref)
				}
				return writeTaskGitRef(ref)
			})
		},
	}

	cmd.Flags().StringVar(&opts.relation, "relation", "", "relation type (e.g. design_doc, closed_by)")
	cmd.Flags().StringVar(&opts.objectType, "type", "", "git object type: commit|tag|branch|path|blob|tree")
	cmd.Flags().StringVar(&opts.objectValue, "value", "", "git object value (sha/ref/path)")
	cmd.Flags().StringVar(&opts.repo, "repo", "", "repository slug (host/owner/repo)")
	cmd.Flags().StringVar(&opts.resolvedCommit, "resolved-commit", "", "resolved commit sha for mutable refs")
	cmd.Flags().StringVar(&opts.note, "note", "", "optional note")
	return cmd
}

func newGitListCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "ls <task-id>",
		Short: "List git references for a task",
		Args:  requireExactlyArgs(1, "task id is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				refs, err := client.ListTaskGitRefs(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				if *jsonOutput {
					return writeJSON(refs)
				}
				for _, ref := range refs {
					if err := writePlain("%s [%s] %s %s\n", ref.ID, ref.Relation, ref.ObjectType, ref.ObjectValue); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
}

func newGitRemoveCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <git-ref-id>",
		Short: "Remove one git reference",
		Args:  requireExactlyArgs(1, "git ref id is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				resp, err := client.DeleteTaskGitRef(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				if *jsonOutput {
					return writeJSON(resp)
				}
				return writePlain("%s\n", args[0])
			})
		},
	}
}

func writeTaskGitRef(ref models.TaskGitRef) error {
	lines := []string{
		fmt.Sprintf("id: %s", ref.ID),
		fmt.Sprintf("task_id: %s", ref.TaskID),
		fmt.Sprintf("repo: %s", ref.Repo),
		fmt.Sprintf("relation: %s", ref.Relation),
		fmt.Sprintf("object_type: %s", ref.ObjectType),
		fmt.Sprintf("object_value: %s", ref.ObjectValue),
	}
	if ref.ResolvedCommit != "" {
		lines = append(lines, fmt.Sprintf("resolved_commit: %s", ref.ResolvedCommit))
	}
	if ref.Note != "" {
		lines = append(lines, fmt.Sprintf("note: %s", ref.Note))
	}
	return writePlain("%s\n", strings.Join(lines, "\n"))
}
