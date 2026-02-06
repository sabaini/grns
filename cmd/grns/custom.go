package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseCustomFlags parses --custom key=value pairs and --custom-json into a
// single map. JSON is parsed first, then key=value pairs overlay on top.
func parseCustomFlags(kvPairs []string, rawJSON string) (map[string]any, error) {
	m := make(map[string]any)

	if rawJSON != "" {
		if err := json.Unmarshal([]byte(rawJSON), &m); err != nil {
			return nil, fmt.Errorf("invalid --custom-json: %w", err)
		}
	}

	for _, pair := range kvPairs {
		idx := strings.IndexByte(pair, '=')
		if idx <= 0 {
			return nil, fmt.Errorf("invalid --custom format %q, expected key=value", pair)
		}
		m[pair[:idx]] = pair[idx+1:]
	}

	return m, nil
}
