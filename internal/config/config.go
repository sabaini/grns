package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	DefaultProjectPrefix = "gr"
	DefaultAPIURL        = "http://127.0.0.1:7333"
	DefaultDBFileName    = ".grns.db"
)

// Config defines runtime configuration for grns.
type Config struct {
	ProjectPrefix string `toml:"project_prefix"`
	APIURL        string `toml:"api_url"`
	DBPath        string `toml:"db_path"`
}

// Default returns default configuration values.
func Default() Config {
	return Config{
		ProjectPrefix: DefaultProjectPrefix,
		APIURL:        DefaultAPIURL,
		DBPath:        "",
	}
}

func loadFile(path string, cfg *Config) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return fmt.Errorf("failed to parse config %s: %w", path, err)
	}
	return nil
}

// Load reads config from global and project files and applies env overrides.
func Load() (*Config, error) {
	cfg := Default()

	if home, err := os.UserHomeDir(); err == nil {
		globalPath := filepath.Join(home, ".grns.toml")
		if err := loadFile(globalPath, &cfg); err != nil {
			return nil, err
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		projectPath := filepath.Join(cwd, ".grns.toml")
		if err := loadFile(projectPath, &cfg); err != nil {
			return nil, err
		}

		if cfg.DBPath == "" {
			cfg.DBPath = filepath.Join(cwd, DefaultDBFileName)
		}
	}

	if apiURL := os.Getenv("GRNS_API_URL"); apiURL != "" {
		cfg.APIURL = apiURL
	}
	if dbPath := os.Getenv("GRNS_DB"); dbPath != "" {
		cfg.DBPath = dbPath
	}

	return &cfg, nil
}
