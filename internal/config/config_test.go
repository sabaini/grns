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

func TestIsAllowedKey(t *testing.T) {
	for _, key := range []string{"project_prefix", "api_url", "db_path"} {
		if !IsAllowedKey(key) {
			t.Fatalf("expected %q to be allowed", key)
		}
	}
	if IsAllowedKey("invalid") {
		t.Fatal("expected 'invalid' to not be allowed")
	}
}

func TestGetKey(t *testing.T) {
	cfg := Config{ProjectPrefix: "xx", APIURL: "http://test:1234", DBPath: "/tmp/test.db"}

	val, err := cfg.Get("project_prefix")
	if err != nil || val != "xx" {
		t.Fatalf("expected 'xx', got %q (err: %v)", val, err)
	}
	val, err = cfg.Get("api_url")
	if err != nil || val != "http://test:1234" {
		t.Fatalf("expected api_url, got %q (err: %v)", val, err)
	}
	val, err = cfg.Get("db_path")
	if err != nil || val != "/tmp/test.db" {
		t.Fatalf("expected db_path, got %q (err: %v)", val, err)
	}
	_, err = cfg.Get("invalid")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestSetKeyCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.toml")
	if err := SetKey(path, "project_prefix", "xx"); err != nil {
		t.Fatalf("set: %v", err)
	}

	cfg := Default()
	if err := loadFile(path, &cfg); err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ProjectPrefix != "xx" {
		t.Fatalf("expected 'xx', got %q", cfg.ProjectPrefix)
	}
}

func TestSetKeyUpdatesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.toml")
	if err := os.WriteFile(path, []byte("project_prefix = \"old\"\napi_url = \"http://keep\"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := SetKey(path, "project_prefix", "new"); err != nil {
		t.Fatalf("set: %v", err)
	}

	cfg := Default()
	if err := loadFile(path, &cfg); err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ProjectPrefix != "new" {
		t.Fatalf("expected 'new', got %q", cfg.ProjectPrefix)
	}
	if cfg.APIURL != "http://keep" {
		t.Fatalf("expected preserved api_url 'http://keep', got %q", cfg.APIURL)
	}
}

func TestSetKeyInvalidKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.toml")
	if err := SetKey(path, "invalid_key", "value"); err == nil {
		t.Fatal("expected error for invalid key")
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
