package main

import (
	"errors"

	"github.com/spf13/cobra"
)

func requireAtLeastArgs(min int, message string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) < min {
			return errors.New(message)
		}
		return nil
	}
}

func requireExactlyArgs(count int, message string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != count {
			return errors.New(message)
		}
		return nil
	}
}

func requireAtLeastOneID(cmd *cobra.Command, args []string) error {
	return requireAtLeastArgs(1, "id is required")(cmd, args)
}
