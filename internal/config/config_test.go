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
	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("expected default log level %q, got %q", DefaultLogLevel, cfg.LogLevel)
	}
	if cfg.Attachments.MaxUploadBytes != DefaultAttachmentMaxUploadBytes {
		t.Fatalf("expected attachment max upload default %d, got %d", DefaultAttachmentMaxUploadBytes, cfg.Attachments.MaxUploadBytes)
	}
	if cfg.Attachments.MultipartMaxMemory != DefaultAttachmentMultipartMemory {
		t.Fatalf("expected attachment multipart default %d, got %d", DefaultAttachmentMultipartMemory, cfg.Attachments.MultipartMaxMemory)
	}
	if !cfg.Attachments.RejectMediaTypeMismatch {
		t.Fatal("expected attachment reject mismatch default true")
	}
	if cfg.Attachments.GCBatchSize != DefaultAttachmentGCBatchSize {
		t.Fatalf("expected attachment gc batch default %d, got %d", DefaultAttachmentGCBatchSize, cfg.Attachments.GCBatchSize)
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".grns.toml")
	if err := os.WriteFile(path, []byte(`project_prefix = "xx"
api_url = "http://localhost:9999"
log_level = "warn"
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
	if cfg.LogLevel != "warn" {
		t.Fatalf("expected log_level 'warn', got %q", cfg.LogLevel)
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
	for _, key := range []string{
		"project_prefix",
		"api_url",
		"db_path",
		"log_level",
		"attachments.max_upload_bytes",
		"attachments.multipart_max_memory",
		"attachments.allowed_media_types",
		"attachments.reject_media_type_mismatch",
		"attachments.gc_batch_size",
	} {
		if !IsAllowedKey(key) {
			t.Fatalf("expected %q to be allowed", key)
		}
	}
	if IsAllowedKey("invalid") {
		t.Fatal("expected 'invalid' to not be allowed")
	}
}

func TestGetKey(t *testing.T) {
	cfg := Config{
		ProjectPrefix: "xx",
		APIURL:        "http://test:1234",
		DBPath:        "/tmp/test.db",
		LogLevel:      "warn",
		Attachments: AttachmentConfig{
			MaxUploadBytes:          123,
			MultipartMaxMemory:      456,
			AllowedMediaTypes:       []string{"application/pdf", "text/plain"},
			RejectMediaTypeMismatch: false,
			GCBatchSize:             789,
		},
	}

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
	val, err = cfg.Get("log_level")
	if err != nil || val != "warn" {
		t.Fatalf("expected log_level, got %q (err: %v)", val, err)
	}
	val, err = cfg.Get("attachments.max_upload_bytes")
	if err != nil || val != "123" {
		t.Fatalf("expected attachments.max_upload_bytes, got %q (err: %v)", val, err)
	}
	val, err = cfg.Get("attachments.multipart_max_memory")
	if err != nil || val != "456" {
		t.Fatalf("expected attachments.multipart_max_memory, got %q (err: %v)", val, err)
	}
	val, err = cfg.Get("attachments.allowed_media_types")
	if err != nil || val != "application/pdf,text/plain" {
		t.Fatalf("expected attachments.allowed_media_types, got %q (err: %v)", val, err)
	}
	val, err = cfg.Get("attachments.reject_media_type_mismatch")
	if err != nil || val != "false" {
		t.Fatalf("expected attachments.reject_media_type_mismatch, got %q (err: %v)", val, err)
	}
	val, err = cfg.Get("attachments.gc_batch_size")
	if err != nil || val != "789" {
		t.Fatalf("expected attachments.gc_batch_size, got %q (err: %v)", val, err)
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

func TestSetKeyLogLevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.toml")
	if err := SetKey(path, "log_level", "error"); err != nil {
		t.Fatalf("set log_level: %v", err)
	}

	cfg := Default()
	if err := loadFile(path, &cfg); err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.LogLevel != "error" {
		t.Fatalf("expected log_level 'error', got %q", cfg.LogLevel)
	}
}

func TestSetKeyInvalidKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.toml")
	if err := SetKey(path, "invalid_key", "value"); err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestSetNestedAttachmentKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attachments.toml")
	if err := SetKey(path, "attachments.gc_batch_size", "321"); err != nil {
		t.Fatalf("set nested key: %v", err)
	}

	cfg := Default()
	if err := loadFile(path, &cfg); err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Attachments.GCBatchSize != 321 {
		t.Fatalf("expected gc_batch_size 321, got %d", cfg.Attachments.GCBatchSize)
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

func TestLoadFallsBackToDefaultLogLevelWhenConfiguredEmpty(t *testing.T) {
	homeDir := t.TempDir()
	workspace := t.TempDir()

	if err := os.WriteFile(filepath.Join(homeDir, ".grns.toml"), []byte("log_level = \"\"\n"), 0o644); err != nil {
		t.Fatalf("write home config: %v", err)
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
	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("expected default log level %q, got %q", DefaultLogLevel, cfg.LogLevel)
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

func TestLoadFallsBackToSnapCommonEnvConfigWhenHomeConfigMissing(t *testing.T) {
	homeDir := t.TempDir()
	workspace := t.TempDir()
	snapCommonDir := t.TempDir()
	snapConfigPath := filepath.Join(snapCommonDir, ".grns.toml")
	if err := os.WriteFile(snapConfigPath, []byte("project_prefix = \"sc\"\n"), 0o644); err != nil {
		t.Fatalf("write snap common env config: %v", err)
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
	t.Setenv("SNAP_COMMON", snapCommonDir)
	t.Setenv("GRNS_TRUST_PROJECT_CONFIG", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ProjectPrefix != "sc" {
		t.Fatalf("expected snap common env config prefix 'sc', got %q", cfg.ProjectPrefix)
	}
}

func TestLoadFallsBackToSnapCommonConfigWhenHomeConfigMissing(t *testing.T) {
	homeDir := t.TempDir()
	workspace := t.TempDir()

	snapConfigPath := filepath.Join(homeDir, "snap", "grns", "common", ".grns.toml")
	if err := os.MkdirAll(filepath.Dir(snapConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir snap config dir: %v", err)
	}
	if err := os.WriteFile(snapConfigPath, []byte("project_prefix = \"sn\"\n"), 0o644); err != nil {
		t.Fatalf("write snap config: %v", err)
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
	t.Setenv("SNAP_COMMON", "")
	t.Setenv("GRNS_TRUST_PROJECT_CONFIG", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ProjectPrefix != "sn" {
		t.Fatalf("expected snap common config prefix 'sn', got %q", cfg.ProjectPrefix)
	}
}

func TestLoadPrefersSnapCommonEnvConfigOverLegacySnapCommonConfig(t *testing.T) {
	homeDir := t.TempDir()
	workspace := t.TempDir()
	snapCommonDir := t.TempDir()

	legacySnapPath := filepath.Join(homeDir, "snap", "grns", "common", ".grns.toml")
	if err := os.MkdirAll(filepath.Dir(legacySnapPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy snap config dir: %v", err)
	}
	if err := os.WriteFile(legacySnapPath, []byte("project_prefix = \"sn\"\n"), 0o644); err != nil {
		t.Fatalf("write legacy snap config: %v", err)
	}
	envSnapPath := filepath.Join(snapCommonDir, ".grns.toml")
	if err := os.WriteFile(envSnapPath, []byte("project_prefix = \"sc\"\n"), 0o644); err != nil {
		t.Fatalf("write snap common env config: %v", err)
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
	t.Setenv("SNAP_COMMON", snapCommonDir)
	t.Setenv("GRNS_TRUST_PROJECT_CONFIG", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ProjectPrefix != "sc" {
		t.Fatalf("expected SNAP_COMMON config prefix 'sc', got %q", cfg.ProjectPrefix)
	}
}

func TestLoadPrefersHomeConfigOverSnapCommonConfig(t *testing.T) {
	homeDir := t.TempDir()
	workspace := t.TempDir()
	snapCommonDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(homeDir, ".grns.toml"), []byte("project_prefix = \"hm\"\n"), 0o644); err != nil {
		t.Fatalf("write home config: %v", err)
	}
	legacySnapPath := filepath.Join(homeDir, "snap", "grns", "common", ".grns.toml")
	if err := os.MkdirAll(filepath.Dir(legacySnapPath), 0o755); err != nil {
		t.Fatalf("mkdir snap config dir: %v", err)
	}
	if err := os.WriteFile(legacySnapPath, []byte("project_prefix = \"sn\"\n"), 0o644); err != nil {
		t.Fatalf("write snap config: %v", err)
	}
	envSnapPath := filepath.Join(snapCommonDir, ".grns.toml")
	if err := os.WriteFile(envSnapPath, []byte("project_prefix = \"sc\"\n"), 0o644); err != nil {
		t.Fatalf("write snap common env config: %v", err)
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
	t.Setenv("SNAP_COMMON", snapCommonDir)
	t.Setenv("GRNS_TRUST_PROJECT_CONFIG", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ProjectPrefix != "hm" {
		t.Fatalf("expected home config prefix 'hm', got %q", cfg.ProjectPrefix)
	}
}

func TestGlobalPathFallsBackToSnapCommonEnvWhenHomeConfigMissing(t *testing.T) {
	homeDir := t.TempDir()
	snapCommonDir := t.TempDir()
	snapConfigPath := filepath.Join(snapCommonDir, ".grns.toml")
	if err := os.WriteFile(snapConfigPath, []byte("project_prefix = \"sc\"\n"), 0o644); err != nil {
		t.Fatalf("write snap common env config: %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("SNAP_COMMON", snapCommonDir)

	path, err := GlobalPath()
	if err != nil {
		t.Fatalf("global path: %v", err)
	}
	if path != snapConfigPath {
		t.Fatalf("expected SNAP_COMMON global path %q, got %q", snapConfigPath, path)
	}
}

func TestGlobalPathFallsBackToSnapCommonWhenHomeConfigMissing(t *testing.T) {
	homeDir := t.TempDir()
	snapConfigPath := filepath.Join(homeDir, "snap", "grns", "common", ".grns.toml")
	if err := os.MkdirAll(filepath.Dir(snapConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir snap config dir: %v", err)
	}
	if err := os.WriteFile(snapConfigPath, []byte("project_prefix = \"sn\"\n"), 0o644); err != nil {
		t.Fatalf("write snap config: %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("SNAP_COMMON", "")

	path, err := GlobalPath()
	if err != nil {
		t.Fatalf("global path: %v", err)
	}
	if path != snapConfigPath {
		t.Fatalf("expected snap common global path %q, got %q", snapConfigPath, path)
	}
}
