package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
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
	} else {
		slog.Debug("api ping failed; attempting auto-spawn", "api_url", cfg.APIURL, "error", err)
	}

	slog.Debug("auto-spawning server", "api_url", cfg.APIURL, "db", cfg.DBPath)
	cmd, logPath, err := startServerProcess(cfg)
	if err != nil {
		return nil, &startupDiagnosticsError{apiURL: cfg.APIURL, logPath: logPath, cause: err}
	}

	if err := waitForServer(client, cfg.APIURL, serverStartTimeout); err != nil {
		slog.Error("server auto-spawn failed", "api_url", cfg.APIURL, "error", err, "log_path", logPath)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, &startupDiagnosticsError{apiURL: cfg.APIURL, logPath: logPath, cause: err}
	}
	slog.Debug("server ready", "api_url", cfg.APIURL)

	cleanup := func() {
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			slog.Debug("failed to kill spawned server", "api_url", cfg.APIURL, "error", err)
		}
		if err := cmd.Wait(); err != nil {
			slog.Debug("spawned server wait returned", "api_url", cfg.APIURL, "error", err)
		}
	}

	return cleanup, nil
}

func startServerProcess(cfg *config.Config) (*exec.Cmd, string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, "", err
	}

	cmd := exec.Command(exe, "srv")
	cmd.Env = append(os.Environ(),
		"GRNS_DB="+cfg.DBPath,
		"GRNS_API_URL="+cfg.APIURL,
	)

	logPath := ""
	logFile, err := serverLogFile()
	if err != nil {
		slog.Warn("could not create server log file", "api_url", cfg.APIURL, "error", err)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
	} else {
		logPath = logFile.Name()
		slog.Debug("server log file", "api_url", cfg.APIURL, "path", logPath)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		slog.Error("failed to start server process", "api_url", cfg.APIURL, "db", cfg.DBPath, "error", err)
		return nil, logPath, err
	}
	slog.Debug("started server process", "api_url", cfg.APIURL, "pid", cmd.Process.Pid)
	return cmd, logPath, nil
}

func waitForServer(client *api.Client, apiURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		err := client.Ping(ctx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if !isConnRefused(err) && !isStartupTimeout(err) {
			slog.Warn("server health check failed with non-transient error", "api_url", apiURL, "error", err)
			return fmt.Errorf("server health check failed: %w", err)
		}
		time.Sleep(serverPollInterval)
	}

	if lastErr != nil {
		return fmt.Errorf("server did not start within %s: last error: %w", timeout, lastErr)
	}
	return fmt.Errorf("server did not start within %s", timeout)
}

func isConnRefused(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}

	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return errors.Is(netErr.Err, syscall.ECONNREFUSED)
	}
	return false
}

func isStartupTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

func serverLogFile() (*os.File, error) {
	dir := filepath.Join(os.TempDir(), "grns")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
}
