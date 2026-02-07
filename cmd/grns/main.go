package main

import (
	"fmt"
	"os"

	"grns/internal/config"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if cfg.TrustedProjectConfigPath != "" {
		fmt.Fprintf(os.Stderr, "warning: using trusted project config from %s\n", cfg.TrustedProjectConfigPath)
	}

	if err := newRootCmd(cfg).Execute(); err != nil {
		for _, line := range formatCLIError(err) {
			fmt.Fprintln(os.Stderr, line)
		}
		os.Exit(1)
	}
}
