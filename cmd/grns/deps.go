package main

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
	"grns/internal/models"
)

func newDepCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	depCmd := &cobra.Command{
		Use:   "dep",
		Short: "Manage dependencies",
	}

	addCmd := &cobra.Command{
		Use:   "add <child> <parent>",
		Short: "Add a dependency",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return errors.New("child and parent ids are required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			depType, _ := cmd.Flags().GetString("type")
			if depType == "" {
				depType = "blocks"
			}
			return withClient(cfg, func(client *api.Client) error {
				resp, err := client.AddDependency(cmd.Context(), api.DepCreateRequest{
					ChildID:  args[0],
					ParentID: args[1],
					Type:     depType,
				})
				if err != nil {
					return err
				}
				if *jsonOutput {
					return writeJSON(resp)
				}
				return writePlain("%s -> %s (%s)\n", args[0], args[1], depType)
			})
		},
	}
	addCmd.Flags().String("type", "", "dependency type")

	depCmd.AddCommand(addCmd)
	return depCmd
}

func parseDeps(value string) ([]models.Dependency, error) {
	items := splitCommaList(value)
	deps := make([]models.Dependency, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, ":", 2)
		if len(parts) == 1 {
			deps = append(deps, models.Dependency{ParentID: parts[0], Type: "blocks"})
			continue
		}
		depType := strings.TrimSpace(parts[0])
		parent := strings.TrimSpace(parts[1])
		if depType == "" || parent == "" {
			return nil, errors.New("invalid dependency format")
		}
		deps = append(deps, models.Dependency{ParentID: parent, Type: depType})
	}
	return deps, nil
}
