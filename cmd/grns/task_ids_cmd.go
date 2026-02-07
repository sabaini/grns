package main

import (
	"context"

	"grns/internal/api"
	"grns/internal/config"
)

type idsMutationFunc func(ctx context.Context, client *api.Client, ids []string) (any, error)

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
