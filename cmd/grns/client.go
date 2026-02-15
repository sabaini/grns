package main

import (
	"grns/internal/api"
	"grns/internal/config"
)

func withClient(cfg *config.Config, fn func(*api.Client) error) error {
	client := api.NewClient(cfg.APIURL)
	client.SetProject(cfg.ProjectPrefix)
	return fn(client)
}
