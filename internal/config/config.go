package config

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	DefaultProjectPrefix = "gr"
	DefaultAPIURL        = "http://127.0.0.1:7333"
	DefaultDBFileName    = ".grns.db"

	DefaultAttachmentMaxUploadBytes  int64 = 100 * 1024 * 1024
	DefaultAttachmentMultipartMemory int64 = 8 * 1024 * 1024
	DefaultAttachmentRejectMismatch        = true
	DefaultAttachmentGCBatchSize           = 500

	configDirEnvKey          = "GRNS_CONFIG_DIR"
	trustProjectConfigEnvKey = "GRNS_TRUST_PROJECT_CONFIG"

	attachmentAllowedMediaTypesEnvKey = "GRNS_ATTACH_ALLOWED_MEDIA_TYPES"
	attachmentRejectMismatchEnvKey    = "GRNS_ATTACH_REJECT_MEDIA_TYPE_MISMATCH"

	snapCommonConfigRelativePath = "snap/grns/common/.grns.toml"
)

// AttachmentConfig defines runtime configuration for attachment handling.
type AttachmentConfig struct {
	MaxUploadBytes          int64    `toml:"max_upload_bytes"`
	MultipartMaxMemory      int64    `toml:"multipart_max_memory"`
	AllowedMediaTypes       []string `toml:"allowed_media_types"`
	RejectMediaTypeMismatch bool     `toml:"reject_media_type_mismatch"`
	GCBatchSize             int      `toml:"gc_batch_size"`
}

// Config defines runtime configuration for grns.
type Config struct {
	ProjectPrefix            string           `toml:"project_prefix"`
	APIURL                   string           `toml:"api_url"`
	DBPath                   string           `toml:"db_path"`
	Attachments              AttachmentConfig `toml:"attachments"`
	TrustedProjectConfigPath string           `toml:"-"`
}

// Default returns default configuration values.
func Default() Config {
	return Config{
		ProjectPrefix: DefaultProjectPrefix,
		APIURL:        DefaultAPIURL,
		DBPath:        "",
		Attachments: AttachmentConfig{
			MaxUploadBytes:          DefaultAttachmentMaxUploadBytes,
			MultipartMaxMemory:      DefaultAttachmentMultipartMemory,
			AllowedMediaTypes:       nil,
			RejectMediaTypeMismatch: DefaultAttachmentRejectMismatch,
			GCBatchSize:             DefaultAttachmentGCBatchSize,
		},
	}
}

func loadFile(path string, cfg *Config) error {
	_, err := loadFileIfExists(path, cfg)
	return err
}

func loadFileIfExists(path string, cfg *Config) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return false, fmt.Errorf("failed to parse config %s: %w", path, err)
	}
	return true, nil
}

func overrideConfigPath() (string, bool) {
	dir := strings.TrimSpace(os.Getenv(configDirEnvKey))
	if dir == "" {
		return "", false
	}
	return filepath.Join(dir, ".grns.toml"), true
}

func trustProjectConfig() bool {
	raw := strings.TrimSpace(os.Getenv(trustProjectConfigEnvKey))
	if raw == "" {
		return false
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return value
}

var allowedKeys = []string{
	"project_prefix",
	"api_url",
	"db_path",
	"attachments.max_upload_bytes",
	"attachments.multipart_max_memory",
	"attachments.allowed_media_types",
	"attachments.reject_media_type_mismatch",
	"attachments.gc_batch_size",
}

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
	case "attachments.max_upload_bytes":
		return strconv.FormatInt(c.Attachments.MaxUploadBytes, 10), nil
	case "attachments.multipart_max_memory":
		return strconv.FormatInt(c.Attachments.MultipartMaxMemory, 10), nil
	case "attachments.allowed_media_types":
		return strings.Join(c.Attachments.AllowedMediaTypes, ","), nil
	case "attachments.reject_media_type_mismatch":
		return strconv.FormatBool(c.Attachments.RejectMediaTypeMismatch), nil
	case "attachments.gc_batch_size":
		return strconv.Itoa(c.Attachments.GCBatchSize), nil
	default:
		return "", fmt.Errorf("unknown key: %s", key)
	}
}

// GlobalPath returns the path to the global config file.
func GlobalPath() (string, error) {
	if path, ok := overrideConfigPath(); ok {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	homePath := filepath.Join(home, ".grns.toml")
	if info, statErr := os.Stat(homePath); statErr == nil && !info.IsDir() {
		return homePath, nil
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", statErr
	}

	snapPath := filepath.Join(home, snapCommonConfigRelativePath)
	if info, statErr := os.Stat(snapPath); statErr == nil && !info.IsDir() {
		return snapPath, nil
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", statErr
	}

	return homePath, nil
}

// ProjectPath returns the path to the project config file.
func ProjectPath() (string, error) {
	if path, ok := overrideConfigPath(); ok {
		return path, nil
	}
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

	data := make(map[string]any)
	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, &data); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	}

	parsedValue, err := parseSetValue(key, value)
	if err != nil {
		return err
	}
	if err := setNestedKey(data, strings.Split(key, "."), parsedValue); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(data)
}

// Load reads config from trusted files and applies env overrides.
func Load() (*Config, error) {
	cfg := Default()

	if overridePath, ok := overrideConfigPath(); ok {
		if err := loadFile(overridePath, &cfg); err != nil {
			return nil, err
		}
	} else {
		if home, err := os.UserHomeDir(); err == nil {
			homePath := filepath.Join(home, ".grns.toml")
			homeLoaded, loadErr := loadFileIfExists(homePath, &cfg)
			if loadErr != nil {
				return nil, loadErr
			}
			if !homeLoaded {
				snapPath := filepath.Join(home, snapCommonConfigRelativePath)
				if err := loadFile(snapPath, &cfg); err != nil {
					return nil, err
				}
			}
		}

		if trustProjectConfig() {
			if cwd, err := os.Getwd(); err == nil {
				projectPath := filepath.Join(cwd, ".grns.toml")
				info, statErr := os.Stat(projectPath)
				switch {
				case statErr == nil && !info.IsDir():
					if err := loadFile(projectPath, &cfg); err != nil {
						return nil, err
					}
					cfg.TrustedProjectConfigPath = projectPath
				case statErr != nil && !os.IsNotExist(statErr):
					return nil, statErr
				}
			}
		}
	}

	if cfg.DBPath == "" {
		if cwd, err := os.Getwd(); err == nil {
			cfg.DBPath = filepath.Join(cwd, DefaultDBFileName)
		}
	}

	if apiURL := os.Getenv("GRNS_API_URL"); apiURL != "" {
		cfg.APIURL = apiURL
	}
	if dbPath := os.Getenv("GRNS_DB"); dbPath != "" {
		cfg.DBPath = dbPath
	}

	if raw := strings.TrimSpace(os.Getenv(attachmentAllowedMediaTypesEnvKey)); raw != "" {
		cfg.Attachments.AllowedMediaTypes = splitCSV(raw)
	}
	if raw := strings.TrimSpace(os.Getenv(attachmentRejectMismatchEnvKey)); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			cfg.Attachments.RejectMediaTypeMismatch = parsed
		}
	}

	cfg.normalizeAttachmentDefaults()

	return &cfg, nil
}

func parseSetValue(key, value string) (any, error) {
	value = strings.TrimSpace(value)
	switch key {
	case "attachments.max_upload_bytes", "attachments.multipart_max_memory":
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("%s must be a positive integer", key)
		}
		return parsed, nil
	case "attachments.gc_batch_size":
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("%s must be a positive integer", key)
		}
		return parsed, nil
	case "attachments.reject_media_type_mismatch":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("%s must be true or false", key)
		}
		return parsed, nil
	case "attachments.allowed_media_types":
		return splitCSV(value), nil
	default:
		return value, nil
	}
}

func setNestedKey(data map[string]any, parts []string, value any) error {
	if len(parts) == 0 {
		return fmt.Errorf("invalid config key")
	}
	if len(parts) == 1 {
		data[parts[0]] = value
		return nil
	}
	childRaw, ok := data[parts[0]]
	if !ok {
		child := map[string]any{}
		data[parts[0]] = child
		return setNestedKey(child, parts[1:], value)
	}
	child, ok := childRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("cannot set nested key %q", strings.Join(parts, "."))
	}
	return setNestedKey(child, parts[1:], value)
}

func splitCSV(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func (c *Config) normalizeAttachmentDefaults() {
	if c.Attachments.MaxUploadBytes <= 0 {
		c.Attachments.MaxUploadBytes = DefaultAttachmentMaxUploadBytes
	}
	if c.Attachments.MultipartMaxMemory <= 0 {
		c.Attachments.MultipartMaxMemory = DefaultAttachmentMultipartMemory
	}
	if c.Attachments.GCBatchSize <= 0 {
		c.Attachments.GCBatchSize = DefaultAttachmentGCBatchSize
	}
	c.Attachments.AllowedMediaTypes = normalizeConfiguredMediaTypes(c.Attachments.AllowedMediaTypes)
}

func normalizeConfiguredMediaTypes(rawValues []string) []string {
	if len(rawValues) == 0 {
		return nil
	}
	out := make([]string, 0, len(rawValues))
	seen := map[string]struct{}{}
	for _, raw := range rawValues {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parsed, _, err := mime.ParseMediaType(raw)
		if err != nil {
			continue
		}
		normalized := strings.ToLower(strings.TrimSpace(parsed))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}
