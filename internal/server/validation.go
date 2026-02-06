package server

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	idRegex = regexp.MustCompile(`^[a-z]{2}-[0-9a-z]{4}$`)

	allowedStatuses = map[string]struct{}{
		"open":       {},
		"in_progress": {},
		"blocked":    {},
		"deferred":   {},
		"closed":     {},
		"tombstone":  {},
		"pinned":     {},
	}
	allowedTypes = map[string]struct{}{
		"bug":     {},
		"feature": {},
		"task":    {},
		"epic":    {},
		"chore":   {},
	}
)

func validateID(id string) bool {
	return idRegex.MatchString(id)
}

func normalizeStatus(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", fmt.Errorf("status is required")
	}
	if _, ok := allowedStatuses[value]; !ok {
		return "", fmt.Errorf("invalid status: %s", value)
	}
	return value, nil
}

func normalizeType(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", fmt.Errorf("type is required")
	}
	if _, ok := allowedTypes[value]; !ok {
		return "", fmt.Errorf("invalid type: %s", value)
	}
	return value, nil
}

func normalizeLabel(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("label is required")
	}
	for _, r := range value {
		if r > unicode.MaxASCII || unicode.IsSpace(r) {
			return "", fmt.Errorf("label must be ascii and non-space")
		}
	}
	return strings.ToLower(value), nil
}

func normalizeLabels(values []string) ([]string, error) {
	labels := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		label, err := normalizeLabel(value)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return labels, nil
}

func normalizePrefix(prefix string) (string, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if len(prefix) != 2 {
		return "", fmt.Errorf("project prefix must be 2 letters")
	}
	for _, r := range prefix {
		if r < 'a' || r > 'z' {
			return "", fmt.Errorf("project prefix must be lowercase letters")
		}
	}
	return prefix, nil
}
