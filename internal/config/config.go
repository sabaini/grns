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

var allowedKeys = []string{"project_prefix", "api_url", "db_path"}

// AllowedKeys returns the set of valid config keys.
func AllowedKeys() []string {
	return allowedKeys
}

// IsAllowedKey checks if a key is a valid config key.
func IsAllowedKey(key string) bool {
	for _, k := range allowedKeys {
		if k == key {
			return true
		}
	}
	return false
}

// Get returns the value of a config key.
func (c *Config) Get(key string) (string, error) {
	switch key {
	case "project_prefix":
		return c.ProjectPrefix, nil
	case "api_url":
		return c.APIURL, nil
	case "db_path":
		return c.DBPath, nil
	default:
		return "", fmt.Errorf("unknown key: %s", key)
	}
}

// GlobalPath returns the path to the global config file.
func GlobalPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".grns.toml"), nil
}

// ProjectPath returns the path to the project config file.
func ProjectPath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, ".grns.toml"), nil
}

// SetKey reads the TOML file at path, sets key=value, and writes it back.
func SetKey(path, key, value string) error {
	if !IsAllowedKey(key) {
		return fmt.Errorf("unknown key: %s", key)
	}

	data := make(map[string]interface{})
	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, &data); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	}

	data[key] = value

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(data)
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
