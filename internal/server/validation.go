package server

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"grns/internal/models"
)

var (
	idRegex           = regexp.MustCompile(`^[a-z]{2}-[0-9a-z]{4}$`)
	attachmentIDRegex = regexp.MustCompile(`^at-[0-9a-z]{4}$`)
	blobIDRegex       = regexp.MustCompile(`^bl-[0-9a-z]{4}$`)
	gitRepoIDRegex    = regexp.MustCompile(`^rp-[0-9a-z]{4}$`)
	gitRefIDRegex     = regexp.MustCompile(`^gf-[0-9a-z]{4}$`)
)

func validateID(id string) bool {
	return idRegex.MatchString(id)
}

func validateAttachmentID(id string) bool {
	return attachmentIDRegex.MatchString(id)
}

func validateBlobID(id string) bool {
	return blobIDRegex.MatchString(id)
}

func validateGitRepoID(id string) bool {
	return gitRepoIDRegex.MatchString(id)
}

func validateGitRefID(id string) bool {
	return gitRefIDRegex.MatchString(id)
}

func normalizeStatus(value string) (string, error) {
	status, err := models.ParseTaskStatus(value)
	if err != nil {
		return "", badRequestCode(err, ErrCodeInvalidStatus)
	}
	return string(status), nil
}

func normalizeType(value string) (string, error) {
	taskType, err := models.ParseTaskType(value)
	if err != nil {
		return "", badRequestCode(err, ErrCodeInvalidType)
	}
	return string(taskType), nil
}

func normalizeLabel(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", badRequestCode(fmt.Errorf("label is required"), ErrCodeMissingRequired)
	}
	for _, r := range value {
		if r > unicode.MaxASCII || unicode.IsSpace(r) {
			return "", badRequestCode(fmt.Errorf("label must be ascii and non-space"), ErrCodeInvalidLabel)
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
