package main

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

type idsMutationFunc func(ctx context.Context, client *api.Client, ids []string) (any, error)

func requireAtLeastOneID(_ *cobra.Command, args []string) error {
	if len(args) == 0 {
		return errors.New("id is required")
	}
	return nil
}

func runIDsMutation(cfg *config.Config, jsonOutput bool, ctx context.Context, ids []string, mutate idsMutationFunc) error {
	return withClient(cfg, func(client *api.Client) error {
		resp, err := mutate(ctx, client, ids)
		if err != nil {
			return err
		}
		if jsonOutput {
			return writeJSON(resp)
		}
		return writePlain("%v\n", ids)
	})
}
