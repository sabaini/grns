package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"grns/internal/config"
)

func TestReadmeSupportedConfigKeysMatchAllowedKeys(t *testing.T) {
	readme := loadReadme(t)

	documented, err := parseDocumentedConfigKeys(readme)
	if err != nil {
		t.Fatalf("parse documented config keys: %v", err)
	}

	allowed := append([]string(nil), config.AllowedKeys()...)
	slices.Sort(documented)
	slices.Sort(allowed)

	if !slices.Equal(documented, allowed) {
		t.Fatalf("README supported config keys mismatch\ndocumented: %v\nallowed:    %v", documented, allowed)
	}
}

func TestReadmeCommandSurfaceMatchesCLILeafCommands(t *testing.T) {
	readme := loadReadme(t)

	documented, err := parseDocumentedCommandPaths(readme)
	if err != nil {
		t.Fatalf("parse documented commands: %v", err)
	}

	cfg := config.Default()
	root := newRootCmd(&cfg)
	actual := collectLeafCommandPaths(root)

	missingInReadme := diff(actual, documented)
	extraInReadme := diff(documented, actual)
	if len(missingInReadme) > 0 || len(extraInReadme) > 0 {
		t.Fatalf("README command surface mismatch\nmissing in README: %v\nextra in README:   %v", missingInReadme, extraInReadme)
	}
}

func TestReadmeRuntimeEnvironmentKeysDocumented(t *testing.T) {
	readme := loadReadme(t)
	documented := parseReadmeEnvKeys(readme)

	required := uniqueSorted([]string{
		"GRNS_API_URL",
		"GRNS_DB",
		"GRNS_HTTP_TIMEOUT",
		"GRNS_LOG_LEVEL",
		"GRNS_CONFIG_DIR",
		"GRNS_TRUST_PROJECT_CONFIG",
		"GRNS_DB_MAX_OPEN_CONNS",
		"GRNS_DB_MAX_IDLE_CONNS",
		"GRNS_DB_CONN_MAX_LIFETIME",
		"GRNS_API_TOKEN",
		"GRNS_ADMIN_TOKEN",
		"GRNS_REQUIRE_AUTH_WITH_USERS",
	})

	missing := diff(required, documented)
	if len(missing) > 0 {
		t.Fatalf("README missing runtime environment keys: %v", missing)
	}
}

func loadReadme(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	data, err := os.ReadFile(filepath.Join(repoRoot, "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	return string(data)
}

func parseDocumentedConfigKeys(readme string) ([]string, error) {
	lines := strings.Split(readme, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "Supported config keys:" {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return nil, fmt.Errorf("missing 'Supported config keys:' section")
	}

	var keys []string
	for i := start; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "- ") {
			if len(keys) > 0 {
				break
			}
			continue
		}
		startTick := strings.Index(line, "`")
		if startTick == -1 {
			return nil, fmt.Errorf("config key bullet missing backticks: %q", line)
		}
		endTick := strings.Index(line[startTick+1:], "`")
		if endTick == -1 {
			return nil, fmt.Errorf("config key bullet has unterminated backticks: %q", line)
		}
		key := line[startTick+1 : startTick+1+endTick]
		if key != "" {
			keys = append(keys, key)
		}
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("no config keys found")
	}

	return uniqueSorted(keys), nil
}

func parseDocumentedCommandPaths(readme string) ([]string, error) {
	idx := strings.Index(readme, "## Commands")
	if idx == -1 {
		return nil, fmt.Errorf("missing '## Commands' section")
	}
	section := readme[idx:]

	fenceStart := strings.Index(section, "```bash")
	if fenceStart == -1 {
		return nil, fmt.Errorf("missing bash code fence in Commands section")
	}
	fenced := section[fenceStart+len("```bash"):]
	fenceEnd := strings.Index(fenced, "```")
	if fenceEnd == -1 {
		return nil, fmt.Errorf("unterminated Commands code fence")
	}
	block := fenced[:fenceEnd]

	var paths []string
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(line, "grns ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		var parts []string
		for _, token := range fields[1:] {
			if strings.HasPrefix(token, "#") || strings.HasPrefix(token, "<") || strings.HasPrefix(token, "[") || strings.HasPrefix(token, "-") || strings.ContainsAny(token, "\"'") {
				break
			}
			parts = append(parts, token)
		}
		if len(parts) > 0 {
			paths = append(paths, strings.Join(parts, " "))
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no command paths parsed from README Commands section")
	}

	return uniqueSorted(paths), nil
}

func parseReadmeEnvKeys(readme string) []string {
	re := regexp.MustCompile(`GRNS_[A-Z0-9_]+`)
	return uniqueSorted(re.FindAllString(readme, -1))
}

func collectLeafCommandPaths(root *cobra.Command) []string {
	paths := make([]string, 0)

	var walk func(cmd *cobra.Command, prefix []string)
	walk = func(cmd *cobra.Command, prefix []string) {
		children := visibleChildren(cmd)
		if len(children) == 0 {
			if len(prefix) > 0 {
				paths = append(paths, strings.Join(prefix, " "))
			}
			return
		}
		for _, child := range children {
			walk(child, append(prefix, child.Name()))
		}
	}

	walk(root, nil)
	return uniqueSorted(paths)
}

func visibleChildren(cmd *cobra.Command) []*cobra.Command {
	children := cmd.Commands()
	filtered := make([]*cobra.Command, 0, len(children))
	for _, child := range children {
		if child.Hidden {
			continue
		}
		switch child.Name() {
		case "help", "completion":
			continue
		}
		filtered = append(filtered, child)
	}
	return filtered
}

func diff(a, b []string) []string {
	setB := make(map[string]struct{}, len(b))
	for _, item := range b {
		setB[item] = struct{}{}
	}
	out := make([]string, 0)
	for _, item := range a {
		if _, ok := setB[item]; !ok {
			out = append(out, item)
		}
	}
	return uniqueSorted(out)
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}
