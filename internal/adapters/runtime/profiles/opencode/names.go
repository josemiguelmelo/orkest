package opencode

import (
	"regexp"
	"strings"
)

var nativeNamePattern = regexp.MustCompile(`[^a-z0-9-]+`)

func normalizeNativeName(value string) string {
	name := nativeNamePattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "orkest-agent"
	}
	return name
}
