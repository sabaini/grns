package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
	"grns/internal/models"
)

type attachCreateOptions struct {
	kind      string
	title     string
	filename  string
	mediaType string
	labels    []string
	expiresAt string
}

type attachLinkOptions struct {
	attachCreateOptions
	url      string
	repoPath string
}

func newAttachCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{Use: "attach", Short: "Manage attachments"}
	cmd.AddCommand(
		newAttachAddCmd(cfg, jsonOutput),
		newAttachAddLinkCmd(cfg, jsonOutput),
		newAttachListCmd(cfg, jsonOutput),
		newAttachShowCmd(cfg, jsonOutput),
		newAttachGetCmd(cfg),
		newAttachRemoveCmd(cfg, jsonOutput),
	)
	return cmd
}

func newAttachAddCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	opts := &attachCreateOptions{}
	cmd := &cobra.Command{
		Use:   "add <task-id> <path>",
		Short: "Upload and attach a file to a task",
		Args:  requireExactlyArgs(2, "task id and path are required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			expiresAt, err := parseOptionalAttachmentTime(opts.expiresAt)
			if err != nil {
				return err
			}
			if strings.TrimSpace(opts.kind) == "" {
				return fmt.Errorf("kind is required")
			}

			path := args[1]
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			req := api.AttachmentUploadRequest{
				Kind:      opts.kind,
				Title:     opts.title,
				Filename:  chooseFirst(strings.TrimSpace(opts.filename), filepath.Base(path)),
				MediaType: opts.mediaType,
				Labels:    opts.labels,
				ExpiresAt: expiresAt,
			}
			return withClient(cfg, func(client *api.Client) error {
				attachment, err := client.CreateTaskAttachment(cmd.Context(), args[0], req, file)
				if err != nil {
					return err
				}
				return writeAttachment(attachment, *jsonOutput)
			})
		},
	}
	bindAttachCreateFlags(cmd, opts)
	return cmd
}

func newAttachAddLinkCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	opts := &attachLinkOptions{}
	cmd := &cobra.Command{
		Use:   "add-link <task-id>",
		Short: "Attach an external URL or repo path",
		Args:  requireExactlyArgs(1, "task id is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			expiresAt, err := parseOptionalAttachmentTime(opts.expiresAt)
			if err != nil {
				return err
			}
			if strings.TrimSpace(opts.kind) == "" {
				return fmt.Errorf("kind is required")
			}

			hasURL := strings.TrimSpace(opts.url) != ""
			hasRepoPath := strings.TrimSpace(opts.repoPath) != ""
			if hasURL == hasRepoPath {
				return fmt.Errorf("exactly one of --url or --repo-path is required")
			}

			req := api.AttachmentCreateLinkRequest{
				Kind:        opts.kind,
				Title:       opts.title,
				Filename:    opts.filename,
				MediaType:   opts.mediaType,
				ExternalURL: opts.url,
				RepoPath:    opts.repoPath,
				Labels:      opts.labels,
				ExpiresAt:   expiresAt,
			}
			return withClient(cfg, func(client *api.Client) error {
				attachment, err := client.CreateTaskAttachmentLink(cmd.Context(), args[0], req)
				if err != nil {
					return err
				}
				return writeAttachment(attachment, *jsonOutput)
			})
		},
	}
	bindAttachCreateFlags(cmd, &opts.attachCreateOptions)
	cmd.Flags().StringVar(&opts.url, "url", "", "external URL")
	cmd.Flags().StringVar(&opts.repoPath, "repo-path", "", "repository-relative path")
	return cmd
}

func newAttachListCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "list <task-id>",
		Short: "List attachments for a task",
		Args:  requireExactlyArgs(1, "task id is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				attachments, err := client.ListTaskAttachments(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				if *jsonOutput {
					return writeJSON(attachments)
				}
				for _, attachment := range attachments {
					name := chooseFirst(attachment.Title, attachment.Filename, attachment.ExternalURL, attachment.RepoPath)
					if err := writePlain("%s [%s] %s\n", attachment.ID, attachment.Kind, name); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
}

func newAttachShowCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "show <attachment-id>",
		Short: "Show one attachment",
		Args:  requireExactlyArgs(1, "attachment id is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				attachment, err := client.GetAttachment(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				return writeAttachment(attachment, *jsonOutput)
			})
		},
	}
}

func newAttachGetCmd(cfg *config.Config) *cobra.Command {
	var (
		outPath string
		force   bool
	)

	cmd := &cobra.Command{
		Use:   "get <attachment-id>",
		Short: "Download managed attachment content",
		Args:  requireExactlyArgs(1, "attachment id is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(outPath) == "" {
				return fmt.Errorf("--output is required")
			}
			if !force {
				if _, err := os.Stat(outPath); err == nil {
					return fmt.Errorf("output file exists (use --force to overwrite)")
				}
			}

			return withClient(cfg, func(client *api.Client) error {
				f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
				if err != nil {
					return err
				}
				defer f.Close()

				if err := client.GetAttachmentContent(cmd.Context(), args[0], f); err != nil {
					return err
				}
				return writePlain("%s\n", outPath)
			})
		},
	}

	cmd.Flags().StringVarP(&outPath, "output", "o", "", "output path")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite output path if it exists")
	return cmd
}

func newAttachRemoveCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <attachment-id>",
		Short: "Remove one attachment",
		Args:  requireExactlyArgs(1, "attachment id is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				resp, err := client.DeleteAttachment(cmd.Context(), args[0])
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

func bindAttachCreateFlags(cmd *cobra.Command, opts *attachCreateOptions) {
	cmd.Flags().StringVar(&opts.kind, "kind", "", "attachment kind")
	cmd.Flags().StringVar(&opts.title, "title", "", "title")
	cmd.Flags().StringVar(&opts.filename, "filename", "", "display filename")
	cmd.Flags().StringVar(&opts.mediaType, "media-type", "", "media type (MIME)")
	cmd.Flags().StringSliceVar(&opts.labels, "label", nil, "label (repeatable)")
	cmd.Flags().StringVar(&opts.expiresAt, "expires-at", "", "expiry time (RFC3339 or YYYY-MM-DD)")
}

func parseOptionalAttachmentTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		t = t.UTC()
		return &t, nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		t = t.UTC()
		return &t, nil
	}
	return nil, fmt.Errorf("invalid expires-at")
}

func writeAttachment(attachment models.Attachment, jsonOutput bool) error {
	if jsonOutput {
		return writeJSON(attachment)
	}
	lines := []string{
		fmt.Sprintf("id: %s", attachment.ID),
		fmt.Sprintf("task_id: %s", attachment.TaskID),
		fmt.Sprintf("kind: %s", attachment.Kind),
		fmt.Sprintf("source_type: %s", attachment.SourceType),
	}
	if attachment.Title != "" {
		lines = append(lines, fmt.Sprintf("title: %s", attachment.Title))
	}
	if attachment.Filename != "" {
		lines = append(lines, fmt.Sprintf("filename: %s", attachment.Filename))
	}
	if attachment.MediaType != "" {
		lines = append(lines, fmt.Sprintf("media_type: %s", attachment.MediaType))
	}
	if attachment.BlobID != "" {
		lines = append(lines, fmt.Sprintf("blob_id: %s", attachment.BlobID))
	}
	if attachment.ExternalURL != "" {
		lines = append(lines, fmt.Sprintf("external_url: %s", attachment.ExternalURL))
	}
	if attachment.RepoPath != "" {
		lines = append(lines, fmt.Sprintf("repo_path: %s", attachment.RepoPath))
	}
	if len(attachment.Labels) > 0 {
		lines = append(lines, fmt.Sprintf("labels: %s", strings.Join(attachment.Labels, ",")))
	}
	return writePlain("%s\n", strings.Join(lines, "\n"))
}

func chooseFirst(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
