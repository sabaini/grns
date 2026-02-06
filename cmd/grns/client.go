package main

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"time"

	"grns/internal/api"
	"grns/internal/config"
)

const (
	serverStartTimeout = 3 * time.Second
	serverPollInterval = 100 * time.Millisecond
)

func withClient(cfg *config.Config, fn func(*api.Client) error) error {
	cleanup, err := ensureServer(cfg)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	client := api.NewClient(cfg.APIURL)
	return fn(client)
}

func ensureServer(cfg *config.Config) (func(), error) {
	client := api.NewClient(cfg.APIURL)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := client.Ping(ctx); err == nil {
		return nil, nil
	}

	cmd, err := startServerProcess(cfg)
	if err != nil {
		return nil, err
	}

	if err := waitForServer(client, serverStartTimeout); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}

	cleanup := func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}

	return cleanup, nil
}

func startServerProcess(cfg *config.Config) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(exe, "srv")
	cmd.Env = append(os.Environ(),
		"GRNS_DB="+cfg.DBPath,
		"GRNS_API_URL="+cfg.APIURL,
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func waitForServer(client *api.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		err := client.Ping(ctx)
		cancel()
		if err == nil {
			return nil
		}
		if !isConnRefused(err) {
			// If port is in use but API is not ours, surface the error.
			return err
		}
		time.Sleep(serverPollInterval)
	}
	return errors.New("server did not start in time")
}

func isConnRefused(err error) bool {
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	return false
}
