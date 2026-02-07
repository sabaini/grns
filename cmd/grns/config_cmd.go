package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"grns/internal/config"
)

func newConfigCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Get or set configuration",
	}

	cmd.AddCommand(newConfigGetCmd(cfg))
	cmd.AddCommand(newConfigSetCmd())
	return cmd
}

func newConfigGetCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if !config.IsAllowedKey(key) {
				return fmt.Errorf("unknown key: %s (allowed: %v)", key, config.AllowedKeys())
			}
			value, err := cfg.Get(key)
			if err != nil {
				return err
			}
			return writePlain("%s\n", value)
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	var global bool

	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]

			var path string
			var err error
			if global {
				path, err = config.GlobalPath()
			} else {
				path, err = config.ProjectPath()
			}
			if err != nil {
				return err
			}

			return config.SetKey(path, key, value)
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "write to global config (~/.grns.toml)")
	return cmd
}
