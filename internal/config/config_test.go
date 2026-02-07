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

func TestConfigDirOverridePaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GRNS_CONFIG_DIR", dir)

	globalPath, err := GlobalPath()
	if err != nil {
		t.Fatalf("global path: %v", err)
	}
	if globalPath != filepath.Join(dir, ".grns.toml") {
		t.Fatalf("unexpected global path: %s", globalPath)
	}

	projectPath, err := ProjectPath()
	if err != nil {
		t.Fatalf("project path: %v", err)
	}
	if projectPath != filepath.Join(dir, ".grns.toml") {
		t.Fatalf("unexpected project path: %s", projectPath)
	}
}

func TestLoadConfigDirOverride(t *testing.T) {
	configDir := t.TempDir()
	cfgPath := filepath.Join(configDir, ".grns.toml")
	if err := os.WriteFile(cfgPath, []byte("project_prefix = \"xy\"\napi_url = \"http://127.0.0.1:9001\"\n"), 0644); err != nil {
		t.Fatalf("write override config: %v", err)
	}

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, ".grns.toml"), []byte("project_prefix = \"zz\"\n"), 0644); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}

	t.Setenv("GRNS_CONFIG_DIR", configDir)
	t.Setenv("GRNS_DB", "")
	t.Setenv("GRNS_API_URL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ProjectPrefix != "xy" {
		t.Fatalf("expected config-dir prefix 'xy', got %q", cfg.ProjectPrefix)
	}
	if cfg.APIURL != "http://127.0.0.1:9001" {
		t.Fatalf("expected config-dir api_url override, got %q", cfg.APIURL)
	}
	if cfg.DBPath != filepath.Join(workspace, DefaultDBFileName) {
		t.Fatalf("expected default workspace db path, got %q", cfg.DBPath)
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

func TestLoadIgnoresProjectConfigByDefault(t *testing.T) {
	homeDir := t.TempDir()
	workspace := t.TempDir()

	if err := os.WriteFile(filepath.Join(homeDir, ".grns.toml"), []byte("project_prefix = \"gh\"\n"), 0o644); err != nil {
		t.Fatalf("write home config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".grns.toml"), []byte("project_prefix = \"pr\"\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("GRNS_TRUST_PROJECT_CONFIG", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ProjectPrefix != "gh" {
		t.Fatalf("expected global config prefix 'gh', got %q", cfg.ProjectPrefix)
	}
	if cfg.TrustedProjectConfigPath != "" {
		t.Fatalf("expected no trusted project config path, got %q", cfg.TrustedProjectConfigPath)
	}
}

func TestLoadAppliesProjectConfigWhenTrusted(t *testing.T) {
	homeDir := t.TempDir()
	workspace := t.TempDir()

	if err := os.WriteFile(filepath.Join(homeDir, ".grns.toml"), []byte("project_prefix = \"gh\"\n"), 0o644); err != nil {
		t.Fatalf("write home config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".grns.toml"), []byte("project_prefix = \"pr\"\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("GRNS_TRUST_PROJECT_CONFIG", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ProjectPrefix != "pr" {
		t.Fatalf("expected trusted project config prefix 'pr', got %q", cfg.ProjectPrefix)
	}
	expectedPath := filepath.Join(workspace, ".grns.toml")
	if cfg.TrustedProjectConfigPath != expectedPath {
		t.Fatalf("expected trusted project config path %q, got %q", expectedPath, cfg.TrustedProjectConfigPath)
	}
}

func TestLoadDoesNotTrustProjectConfigOnInvalidEnvValue(t *testing.T) {
	homeDir := t.TempDir()
	workspace := t.TempDir()

	if err := os.WriteFile(filepath.Join(homeDir, ".grns.toml"), []byte("project_prefix = \"gh\"\n"), 0o644); err != nil {
		t.Fatalf("write home config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".grns.toml"), []byte("project_prefix = \"pr\"\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("GRNS_TRUST_PROJECT_CONFIG", "definitely-not-bool")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ProjectPrefix != "gh" {
		t.Fatalf("expected global config prefix 'gh' with invalid trust env, got %q", cfg.ProjectPrefix)
	}
	if cfg.TrustedProjectConfigPath != "" {
		t.Fatalf("expected no trusted project config path with invalid trust env, got %q", cfg.TrustedProjectConfigPath)
	}
}
