// Package configvalidation checks shared resolved configuration values without domain dependencies.
package configvalidation

import (
	"encoding/json"
	"net/url"
	"path"
	"regexp"
	"strings"

	runtimecatalog "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

// Issue describes a rejected field without retaining its sensitive value.
type Issue struct{ Field, Reason string }

var (
	skillIDPattern         = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func ValidSkillID(value string) bool { return skillIDPattern.MatchString(value) }

func Required(fields ...[2]string) *Issue {
	for _, field := range fields {
		if strings.TrimSpace(field[1]) == "" {
			return &Issue{Field: field[0]}
		}
	}
	return nil
}

func Runtime(value runtimecatalog.Runtime, provider, policy string) *Issue {
	if !value.Available() {
		return &Issue{Field: "runtime", Reason: "is unsupported"}
	}
	if policy != "" && policy != "ask" && policy != "autonomous" {
		return &Issue{Field: "agent.permissionPolicy", Reason: "is unsupported"}
	}
	if definition, ok := runtimecatalog.ProviderFor(value); ok {
		if err := definition.ValidateDefinition(runtimecatalog.Definition{ModelProvider: provider}); err != nil {
			return &Issue{Field: "model.provider", Reason: "is invalid for runtime"}
		}
	}
	return nil
}

func MCPServer(id, name, endpoint string, headers map[string]string) *Issue {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" {
		return &Issue{Field: "mcpServers", Reason: "must include an ID and name"}
	}
	if !httpURL(endpoint, false) {
		return &Issue{Field: "mcpServers", Reason: "must use HTTP or HTTPS URLs without user information"}
	}
	for name, value := range headers {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name+value, "\r\n") {
			return &Issue{Field: "mcpServers.headers", Reason: "must have nonempty names and no line breaks"}
		}
	}
	return nil
}

func Skill(id, definition string, duplicate bool) *Issue {
	if !ValidSkillID(id) {
		return &Issue{Field: "skills", Reason: "must use lowercase alphanumeric IDs separated by single hyphens"}
	}
	if strings.TrimSpace(definition) == "" {
		return &Issue{Field: "skills", Reason: "must include a nonempty definition"}
	}
	if duplicate {
		return &Issue{Field: "skills", Reason: "must have unique IDs"}
	}
	return nil
}

func GitCredential(provider, token, path, repositoryURL string, providerConfig json.RawMessage) *Issue {
	invalid := &Issue{Field: "gitCredentials", Reason: "must contain valid HTTPS repository credentials"}
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(token) == "" {
		return invalid
	}
	if !httpURL(repositoryURL, true) {
		return invalid
	}
	if strings.TrimSpace(path) == "" {
		parsed, _ := url.Parse(repositoryURL)
		if !strings.HasSuffix(parsed.Path, "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return invalid
		}
	}
	if strings.ContainsAny(token, "\x00\r\n") {
		return invalid
	}
	if len(providerConfig) > 0 && !json.Valid(providerConfig) {
		return invalid
	}
	return nil
}

func Environment(values map[string]string) *Issue {
	for name, value := range values {
		if !environmentNamePattern.MatchString(name) || strings.ContainsRune(value, '\x00') {
			return &Issue{Field: "environment", Reason: "contains an invalid name or value"}
		}
		if strings.HasPrefix(name, "GIT_CONFIG_") {
			return &Issue{Field: "environment", Reason: "contains a reserved Git configuration variable"}
		}
		if strings.HasPrefix(name, "ORKEST_GH_") {
			return &Issue{Field: "environment", Reason: "contains a reserved GitHub CLI variable"}
		}
		if strings.HasPrefix(name, "ORKEST_GITLAB_") {
			return &Issue{Field: "environment", Reason: "contains a reserved GitLab CLI variable"}
		}
	}
	return nil
}

func Repository(id, provider, endpoint, integrationID, providerID, branch string, duplicate bool) *Issue {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(provider) == "" || strings.TrimSpace(endpoint) == "" {
		return &Issue{Field: "repositories", Reason: "must include an ID, provider name, and URL"}
	}
	if integrationID != "" && (strings.TrimSpace(providerID) == "" || strings.TrimSpace(branch) == "") {
		return &Issue{Field: "repositories", Reason: "integrated repositories must include provider identity and default branch"}
	}
	if duplicate {
		return &Issue{Field: "repositories", Reason: "must have unique IDs"}
	}
	return nil
}

func httpURL(value string, httpsOnly bool) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Host == "" || parsed.User != nil {
		return false
	}
	return parsed.Scheme == "https" || (!httpsOnly && parsed.Scheme == "http")
}

// MCPConnection validates either supported transport without including secret values in errors.
func MCPConnection(id, name, transport, endpoint, command string, args []string, headers, environment map[string]string) *Issue {
	if transport == "" || transport == "http" {
		if command != "" || len(args) != 0 || len(environment) != 0 {
			return &Issue{Field: "mcpServers", Reason: "HTTP connections cannot include stdio fields"}
		}
		return MCPServer(id, name, endpoint, headers)
	}
	if transport != "stdio" {
		return &Issue{Field: "mcpServers.transport", Reason: "must be http or stdio"}
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" {
		return &Issue{Field: "mcpServers", Reason: "must include an ID and name"}
	}
	if endpoint != "" || len(headers) != 0 {
		return &Issue{Field: "mcpServers", Reason: "stdio connections cannot include HTTP fields"}
	}
	if !path.IsAbs(command) || strings.ContainsAny(command, "\x00\r\n") || len(command) > 4096 || len(args) > 100 {
		return &Issue{Field: "mcpServers.command", Reason: "must name an absolute executable path and at most 100 arguments"}
	}
	for _, arg := range args {
		if strings.ContainsRune(arg, '\x00') || len(arg) > 8192 {
			return &Issue{Field: "mcpServers.args", Reason: "contains an invalid argument"}
		}
	}
	if len(environment) > 50 {
		return &Issue{Field: "mcpServers.environment", Reason: "must contain at most 50 variables"}
	}
	for key, value := range environment {
		if !environmentNamePattern.MatchString(key) || len(key) > 256 || len(value) > 8192 || strings.ContainsRune(value, '\x00') {
			return &Issue{Field: "mcpServers.environment", Reason: "contains an invalid name or value"}
		}
	}
	return nil
}
