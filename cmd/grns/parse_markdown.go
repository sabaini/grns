package main

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"grns/internal/api"
	"grns/internal/models"
)

var listItemRegex = regexp.MustCompile(`^\s*[-*]\s+(.*)$`)

func parseMarkdown(input string) (map[string]any, []string, error) {
	frontMatter := map[string]any{}
	content := input

	lines := strings.Split(input, "\n")
	if len(lines) >= 3 && strings.TrimSpace(lines[0]) == "---" {
		end := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				end = i
				break
			}
		}
		if end == -1 {
			return nil, nil, fmt.Errorf("front matter not closed")
		}
		frontText := strings.Join(lines[1:end], "\n")
		if err := yaml.Unmarshal([]byte(frontText), &frontMatter); err != nil {
			return nil, nil, err
		}
		content = strings.Join(lines[end+1:], "\n")
	}

	items := []string{}
	for _, line := range strings.Split(content, "\n") {
		match := listItemRegex.FindStringSubmatch(line)
		if len(match) == 2 {
			item := strings.TrimSpace(match[1])
			if item != "" {
				items = append(items, item)
			}
		}
	}

	return frontMatter, items, nil
}

func frontMatterToRequest(frontMatter map[string]any) (api.TaskCreateRequest, error) {
	req := api.TaskCreateRequest{}

	if value, ok := frontMatter["type"].(string); ok {
		req.Type = &value
	}
	if value, ok := frontMatter["priority"]; ok {
		switch v := value.(type) {
		case int:
			req.Priority = &v
		case int64:
			converted := int(v)
			req.Priority = &converted
		case float64:
			converted := int(v)
			req.Priority = &converted
		}
	}
	if value, ok := frontMatter["description"].(string); ok {
		req.Description = &value
	}
	if value, ok := frontMatter["spec_id"].(string); ok {
		req.SpecID = &value
	}
	if value, ok := frontMatter["status"].(string); ok {
		req.Status = &value
	}
	if value, ok := frontMatter["parent_id"].(string); ok {
		req.ParentID = &value
	}
	if value, ok := frontMatter["assignee"].(string); ok {
		req.Assignee = &value
	}
	if value, ok := frontMatter["notes"].(string); ok {
		req.Notes = &value
	}
	if value, ok := frontMatter["design"].(string); ok {
		req.Design = &value
	}
	if value, ok := frontMatter["acceptance_criteria"].(string); ok {
		req.AcceptanceCriteria = &value
	}
	if value, ok := frontMatter["source_repo"].(string); ok {
		req.SourceRepo = &value
	}
	if value, ok := frontMatter["labels"]; ok {
		req.Labels = toStringSlice(value)
	}
	if value, ok := frontMatter["deps"]; ok {
		deps, err := parseDepsAny(value)
		if err != nil {
			return req, err
		}
		req.Deps = deps
	}

	return req, nil
}

func toStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return splitCommaList(v)
	}
	return nil
}

func parseDepsAny(value any) ([]models.Dependency, error) {
	switch v := value.(type) {
	case string:
		return parseDeps(v)
	case []any:
		values := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				values = append(values, s)
			}
		}
		return parseDeps(strings.Join(values, ","))
	case []string:
		return parseDeps(strings.Join(v, ","))
	}
	return nil, nil
}
