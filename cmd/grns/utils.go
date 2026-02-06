package main

import (
	"net/url"
	"strconv"
	"strings"
)

func intToString(value int) string {
	return strconv.Itoa(value)
}

func setIfNotEmpty(values url.Values, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	values.Set(key, value)
}

func splitCommaList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
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

