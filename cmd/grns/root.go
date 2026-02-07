package main

import (
	"github.com/spf13/cobra"

	"grns/internal/config"
)

func newRootCmd(cfg *config.Config) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:           "grns",
		Short:         "Grns is a lightweight task tracker and memory system for agents",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.Version = version
	cmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output JSON")

	cmd.AddCommand(
		newSrvCmd(cfg),
		newCreateCmd(cfg, &jsonOutput),
		newShowCmd(cfg, &jsonOutput),
		newUpdateCmd(cfg, &jsonOutput),
		newListCmd(cfg, &jsonOutput),
		newReadyCmd(cfg, &jsonOutput),
		newStaleCmd(cfg, &jsonOutput),
		newCloseCmd(cfg, &jsonOutput),
		newReopenCmd(cfg, &jsonOutput),
		newDepCmd(cfg, &jsonOutput),
		newLabelCmd(cfg, &jsonOutput),
		newMigrateCmd(cfg, &jsonOutput),
		newInfoCmd(cfg, &jsonOutput),
		newAdminCmd(cfg, &jsonOutput),
		newExportCmd(cfg, &jsonOutput),
		newImportCmd(cfg, &jsonOutput),
		newConfigCmd(cfg),
	)

	return cmd
}
