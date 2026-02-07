package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"grns/internal/config"
)

func newRootCmd(cfg *config.Config) *cobra.Command {
	var jsonOutput bool
	var logLevel string

	cmd := &cobra.Command{
		Use:           "grns",
		Short:         "Grns is a lightweight task tracker and memory system for agents",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			warning, err := configureLoggerForCLI(logLevel)
			if err != nil {
				return err
			}
			if warning != "" {
				fmt.Fprintln(os.Stderr, warning)
			}
			return nil
		},
	}

	cmd.Version = version
	cmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output JSON")
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level: debug|info|warn|error (overrides GRNS_LOG_LEVEL)")

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
