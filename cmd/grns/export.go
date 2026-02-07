package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

func newExportCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export all tasks as JSONL",
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOutput != nil && *jsonOutput {
				return fmt.Errorf("export always emits NDJSON; remove --json")
			}
			return withClient(cfg, func(client *api.Client) error {
				w := os.Stdout
				if outputPath != "" {
					f, err := os.Create(outputPath)
					if err != nil {
						return err
					}
					defer f.Close()
					w = f
				}
				return client.Export(cmd.Context(), w)
			})
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "output file (default: stdout)")

	return cmd
}
