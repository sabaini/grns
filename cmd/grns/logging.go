package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

const logLevelEnvKey = "GRNS_LOG_LEVEL"

func configureLoggerForCLI(flagLevel string) (string, error) {
	envLevel := os.Getenv(logLevelEnvKey)
	rawLevel, source := selectedLogLevel(flagLevel, envLevel)
	if err := configureDefaultLogger(rawLevel); err != nil {
		if source == "flag" {
			return "", fmt.Errorf("invalid --log-level %q", flagLevel)
		}
		_ = configureDefaultLogger("")
		return fmt.Sprintf("warning: invalid %s=%q; defaulting to info", logLevelEnvKey, envLevel), nil
	}
	return "", nil
}

func selectedLogLevel(flagLevel, envLevel string) (string, string) {
	if strings.TrimSpace(flagLevel) != "" {
		return flagLevel, "flag"
	}
	if strings.TrimSpace(envLevel) != "" {
		return envLevel, "env"
	}
	return "", "default"
}

func configureDefaultLogger(rawLevel string) error {
	level, err := parseLogLevel(rawLevel)
	if err != nil {
		return err
	}
	slog.SetDefault(newLogger(level))
	return nil
}

func parseLogLevel(raw string) (slog.Level, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return slog.LevelInfo, nil
	}
	if strings.EqualFold(value, "warning") {
		value = "warn"
	}

	if numeric, err := strconv.Atoi(value); err == nil {
		return slog.Level(numeric), nil
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(value)); err != nil {
		return slog.LevelInfo, fmt.Errorf("invalid log level %q", raw)
	}
	return level, nil
}

func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}
