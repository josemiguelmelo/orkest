package configvalidation

import (
	"encoding/json"
	"testing"
)

func TestCredentialValidationDoesNotExposeValues(t *testing.T) {
	for _, test := range []struct {
		name, url, token string
		config           json.RawMessage
		valid            bool
	}{
		{"valid", "https://example.test/repo", "sensitive-token", nil, true},
		{"valid config", "https://example.test/repo", "sensitive-token", json.RawMessage(`{"host":"example.test"}`), true},
		{"http", "http://example.test/repo", "sensitive-token", nil, false},
		{"userinfo", "https://user:secret@example.test/repo", "sensitive-token", nil, false},
		{"missing host", "https:///repo", "sensitive-token", nil, false},
		{"blank token", "https://example.test/repo", " ", nil, false},
		{"token newline", "https://example.test/repo", "secret\nheader", nil, false},
		{"invalid config", "https://example.test/repo", "sensitive-token", json.RawMessage(`{`), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			issue := GitCredential("github", test.token, "repositories/github/repo", test.url, test.config)
			if (issue == nil) != test.valid {
				t.Fatal("unexpected credential validation result")
			}
			if issue != nil && (issue.Field != "gitCredentials" || issue.Reason != "must contain valid HTTPS repository credentials") {
				t.Fatal("credential error classification changed")
			}
		})
	}
}

func TestReservedEnvironmentValidation(t *testing.T) {
	for _, test := range []struct{ name, value, reason string }{
		{"MODEL_TOKEN", "secret", ""},
		{"bad-name", "secret", "contains an invalid name or value"},
		{"MODEL_TOKEN", "secret\x00", "contains an invalid name or value"},
		{"GIT_CONFIG_COUNT", "secret", "contains a reserved Git configuration variable"},
		{"ORKEST_GH_TOKEN", "secret", "contains a reserved GitHub CLI variable"},
		{"ORKEST_GITLAB_TOKEN", "secret", "contains a reserved GitLab CLI variable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			issue := Environment(map[string]string{test.name: test.value})
			if test.reason == "" {
				if issue != nil {
					t.Fatal("valid environment rejected")
				}
				return
			}
			if issue == nil || issue.Field != "environment" || issue.Reason != test.reason {
				t.Fatal("environment error classification changed")
			}
		})
	}
}

func TestValidationPrecedence(t *testing.T) {
	issue := Required([2]string{"organizationId", " "}, [2]string{"runId", ""})
	if issue == nil || issue.Field != "organizationId" || issue.Reason != "" {
		t.Fatal("required-field ordering changed")
	}
	if issue := Skill("invalid ID", "", true); issue == nil || issue.Reason != "must use lowercase alphanumeric IDs separated by single hyphens" {
		t.Fatal("skill validation precedence changed")
	}
	if issue := Repository("", "", "", "integration", "", "", true); issue == nil || issue.Reason != "must include an ID, provider name, and URL" {
		t.Fatal("repository validation precedence changed")
	}
	if issue := MCPServer("server", "Server", "https://example.test", map[string]string{"Authorization": "secret\r\nheader"}); issue == nil || issue.Field != "mcpServers.headers" {
		t.Fatal("unsafe header accepted")
	}
}
