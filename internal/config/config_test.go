package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.ProjectPrefix != "gr" {
		t.Fatalf("expected prefix 'gr', got %q", cfg.ProjectPrefix)
	}
	if cfg.APIURL != "http://127.0.0.1:7333" {
		t.Fatalf("expected default API URL, got %q", cfg.APIURL)
	}
	if cfg.DBPath != "" {
		t.Fatalf("expected empty db path, got %q", cfg.DBPath)
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".grns.toml")
	if err := os.WriteFile(path, []byte(`project_prefix = "xx"
api_url = "http://localhost:9999"
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := Default()
	if err := loadFile(path, &cfg); err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ProjectPrefix != "xx" {
		t.Fatalf("expected prefix 'xx', got %q", cfg.ProjectPrefix)
	}
	if cfg.APIURL != "http://localhost:9999" {
		t.Fatalf("expected api_url 'http://localhost:9999', got %q", cfg.APIURL)
	}
}

func TestLoadFileMissing(t *testing.T) {
	cfg := Default()
	if err := loadFile("/nonexistent/path/.grns.toml", &cfg); err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if cfg.ProjectPrefix != "gr" {
		t.Fatalf("defaults should be preserved")
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("GRNS_API_URL", "http://example.com:8080")
	t.Setenv("GRNS_DB", "/tmp/override.db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.APIURL != "http://example.com:8080" {
		t.Fatalf("expected env override for API URL, got %q", cfg.APIURL)
	}
	if cfg.DBPath != "/tmp/override.db" {
		t.Fatalf("expected env override for DB path, got %q", cfg.DBPath)
	}
}
