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
	depCmd.AddCommand(
		newDepAddCmd(cfg, jsonOutput),
		newDepTreeCmd(cfg, jsonOutput),
	)
	return depCmd
}

func newDepAddCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <child> <parent>",
		Short: "Add a dependency",
		Args:  requireAtLeastArgs(2, "child and parent ids are required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			depType, _ := cmd.Flags().GetString("type")
			if depType == "" {
				depType = string(models.DependencyBlocks)
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
	cmd.Flags().String("type", "", "dependency type")
	return cmd
}

func newDepTreeCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "tree <id>",
		Short: "Show full dependency tree for a task",
		Args:  requireAtLeastArgs(1, "task id is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				resp, err := client.DependencyTree(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				if *jsonOutput {
					return writeJSON(resp)
				}
				return writeDependencyTree(args[0], resp.Nodes)
			})
		},
	}
}

func writeDependencyTree(rootID string, nodes []models.DepTreeNode) error {
	if len(nodes) == 0 {
		return writePlain("No dependencies for %s\n", rootID)
	}
	for _, node := range nodes {
		indent := strings.Repeat("  ", node.Depth)
		arrow := "^"
		if node.Direction == "downstream" {
			arrow = "v"
		}
		if err := writePlain("%s%s %s [%s] %s (%s)\n", indent, arrow, node.ID, node.Status, node.Title, node.DepType); err != nil {
			return err
		}
	}
	return nil
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
			deps = append(deps, models.Dependency{ParentID: parts[0], Type: string(models.DependencyBlocks)})
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
