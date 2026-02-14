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
	DefaultLogLevel      = "debug"

	DefaultAttachmentMaxUploadBytes  int64 = 100 * 1024 * 1024
	DefaultAttachmentMultipartMemory int64 = 8 * 1024 * 1024
	DefaultAttachmentRejectMismatch        = true
	DefaultAttachmentGCBatchSize           = 500

	configDirEnvKey          = "GRNS_CONFIG_DIR"
	trustProjectConfigEnvKey = "GRNS_TRUST_PROJECT_CONFIG"
	snapCommonEnvKey         = "SNAP_COMMON"

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
	ProjectPrefix            string            `toml:"project_prefix"`
	APIURL                   string            `toml:"api_url"`
	DBPath                   string            `toml:"db_path"`
	LogLevel                 string            `toml:"log_level"`
	Attachments              AttachmentConfig  `toml:"attachments"`
	TrustedProjectConfigPath string            `toml:"-"`
	ValueSources             map[string]string `toml:"-"`
	LoadedConfigPaths        []string          `toml:"-"`
}

// Default returns default configuration values.
func Default() Config {
	return Config{
		ProjectPrefix:     DefaultProjectPrefix,
		APIURL:            DefaultAPIURL,
		DBPath:            "",
		LogLevel:          DefaultLogLevel,
		ValueSources:      defaultValueSources(),
		LoadedConfigPaths: nil,
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
	_, _, err := loadFileIfExistsWithKeys(path, cfg)
	return err
}

func loadFileIfExists(path string, cfg *Config) (bool, error) {
	loaded, _, err := loadFileIfExistsWithKeys(path, cfg)
	return loaded, err
}

func loadFileIfExistsWithKeys(path string, cfg *Config) (bool, []string, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	if info.IsDir() {
		return false, nil, nil
	}
	metadata, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return false, nil, fmt.Errorf("failed to parse config %s: %w", path, err)
	}
	return true, metadataKeys(metadata.Keys()), nil
}

func metadataKeys(keys []toml.Key) []string {
	if len(keys) == 0 {
		return nil
	}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if len(key) == 0 {
			continue
		}
		out = append(out, key.String())
	}
	return out
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

func snapCommonConfigPath() (string, bool) {
	dir := strings.TrimSpace(os.Getenv(snapCommonEnvKey))
	if dir == "" {
		return "", false
	}
	return filepath.Join(dir, ".grns.toml"), true
}

func snapFallbackConfigPaths(home string) []string {
	paths := make([]string, 0, 2)
	seen := map[string]struct{}{}
	addPath := func(path string) {
		if path == "" {
			return
		}
		cleanPath := filepath.Clean(path)
		if _, ok := seen[cleanPath]; ok {
			return
		}
		seen[cleanPath] = struct{}{}
		paths = append(paths, path)
	}

	if snapPath, ok := snapCommonConfigPath(); ok {
		addPath(snapPath)
	}
	addPath(filepath.Join(home, snapCommonConfigRelativePath))

	return paths
}

func firstExistingFilePath(paths []string) (string, bool, error) {
	for _, path := range paths {
		info, err := os.Stat(path)
		switch {
		case err == nil && !info.IsDir():
			return path, true, nil
		case err == nil && info.IsDir():
			continue
		case err != nil && os.IsNotExist(err):
			continue
		default:
			return "", false, err
		}
	}
	return "", false, nil
}

var allowedKeys = []string{
	"project_prefix",
	"api_url",
	"db_path",
	"log_level",
	"attachments.max_upload_bytes",
	"attachments.multipart_max_memory",
	"attachments.allowed_media_types",
	"attachments.reject_media_type_mismatch",
	"attachments.gc_batch_size",
}

func defaultValueSources() map[string]string {
	sources := make(map[string]string, len(allowedKeys))
	for _, key := range allowedKeys {
		sources[key] = "default"
	}
	return sources
}

func (c *Config) setSource(key, source string) {
	if c == nil || key == "" || !IsAllowedKey(key) {
		return
	}
	if c.ValueSources == nil {
		c.ValueSources = defaultValueSources()
	}
	c.ValueSources[key] = source
}

func (c *Config) setSources(keys []string, source string) {
	for _, key := range keys {
		c.setSource(key, source)
	}
}

func (c *Config) addLoadedConfigPath(path string) {
	if c == nil {
		return
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	for _, existing := range c.LoadedConfigPaths {
		if existing == path {
			return
		}
	}
	c.LoadedConfigPaths = append(c.LoadedConfigPaths, path)
}

// Source returns where a given config key was resolved from.
func (c *Config) Source(key string) string {
	if c == nil || !IsAllowedKey(key) {
		return ""
	}
	if c.ValueSources == nil {
		return "default"
	}
	source := strings.TrimSpace(c.ValueSources[key])
	if source == "" {
		return "default"
	}
	return source
}

// LoadedPaths returns config files that were successfully loaded.
func (c *Config) LoadedPaths() []string {
	if c == nil || len(c.LoadedConfigPaths) == 0 {
		return nil
	}
	out := make([]string, len(c.LoadedConfigPaths))
	copy(out, c.LoadedConfigPaths)
	return out
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
	case "log_level":
		return c.LogLevel, nil
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
	candidatePaths := []string{homePath}
	candidatePaths = append(candidatePaths, snapFallbackConfigPaths(home)...)

	existingPath, found, err := firstExistingFilePath(candidatePaths)
	if err != nil {
		return "", err
	}
	if found {
		return existingPath, nil
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
		loaded, keys, err := loadFileIfExistsWithKeys(overridePath, &cfg)
		if err != nil {
			return nil, err
		}
		if loaded {
			cfg.setSources(keys, "file:"+overridePath)
			cfg.addLoadedConfigPath(overridePath)
		}
	} else {
		if home, err := os.UserHomeDir(); err == nil {
			homePath := filepath.Join(home, ".grns.toml")
			loaded, keys, loadErr := loadFileIfExistsWithKeys(homePath, &cfg)
			if loadErr != nil {
				return nil, loadErr
			}
			if loaded {
				cfg.setSources(keys, "file:"+homePath)
				cfg.addLoadedConfigPath(homePath)
			}
			if !loaded {
				for _, path := range snapFallbackConfigPaths(home) {
					loaded, keys, loadErr = loadFileIfExistsWithKeys(path, &cfg)
					if loadErr != nil {
						return nil, loadErr
					}
					if loaded {
						cfg.setSources(keys, "file:"+path)
						cfg.addLoadedConfigPath(path)
						break
					}
				}
			}
		}

		if trustProjectConfig() {
			if cwd, err := os.Getwd(); err == nil {
				projectPath := filepath.Join(cwd, ".grns.toml")
				info, statErr := os.Stat(projectPath)
				switch {
				case statErr == nil && !info.IsDir():
					loaded, keys, err := loadFileIfExistsWithKeys(projectPath, &cfg)
					if err != nil {
						return nil, err
					}
					if loaded {
						cfg.setSources(keys, "file:"+projectPath)
						cfg.addLoadedConfigPath(projectPath)
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
			cfg.setSource("db_path", "default:workspace")
		}
	}
	if strings.TrimSpace(cfg.LogLevel) == "" {
		cfg.LogLevel = DefaultLogLevel
		cfg.setSource("log_level", "default")
	}

	if apiURL := strings.TrimSpace(os.Getenv("GRNS_API_URL")); apiURL != "" {
		cfg.APIURL = apiURL
		cfg.setSource("api_url", "env:GRNS_API_URL")
	}
	if dbPath := strings.TrimSpace(os.Getenv("GRNS_DB")); dbPath != "" {
		cfg.DBPath = dbPath
		cfg.setSource("db_path", "env:GRNS_DB")
	}

	if raw := strings.TrimSpace(os.Getenv(attachmentAllowedMediaTypesEnvKey)); raw != "" {
		cfg.Attachments.AllowedMediaTypes = splitCSV(raw)
		cfg.setSource("attachments.allowed_media_types", "env:"+attachmentAllowedMediaTypesEnvKey)
	}
	if raw := strings.TrimSpace(os.Getenv(attachmentRejectMismatchEnvKey)); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			cfg.Attachments.RejectMediaTypeMismatch = parsed
			cfg.setSource("attachments.reject_media_type_mismatch", "env:"+attachmentRejectMismatchEnvKey)
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
