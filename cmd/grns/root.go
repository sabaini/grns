package main

import (
	"github.com/spf13/cobra"

	"grns/internal/config"
)

func newRootCmd(cfg *config.Config) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "grns",
		Short: "Grns is a lightweight issue tracker and memory system for agents",
	}

	cmd.Version = "0.0.0"
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
	)

	return cmd
}
