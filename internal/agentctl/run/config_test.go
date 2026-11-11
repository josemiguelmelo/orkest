package run

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
)

func mutableRunConfig() configmodel.RunConfig {
	config := runConfig()
	config.MCPServers = []configmodel.MCPServer{{ID: "mcp", Name: "MCP", URL: "https://example.com/mcp", Headers: map[string]string{"Authorization": "private-header"}, Transport: "http"}}
	config.Skills = []configmodel.Skill{{ID: "work", SkillMD: "Work instructions"}}
	config.Repositories = []configmodel.WorkspaceRepository{{ID: "repo", ProviderName: "github", URL: "https://example.com/repo"}}
	return config
}

func TestConfigureOwnsMutableInputs(t *testing.T) {
	controller := NewRunController(context.Background(), nil, "", nil)
	defer func() { _ = controller.Close() }()
	config := mutableRunConfig()
	original := config.Clone()
	if err := controller.Configure(config); err != nil {
		t.Fatal(err)
	}
	config.MCPServers[0].URL = "https://changed.example.com/mcp"
	config.MCPServers[0].Headers["Authorization"] = "changed-header"
	config.Skills[0].SkillMD = "Changed instructions"
	config.Repositories[0].URL = "https://changed.example.com/repo"
	if err := controller.Configure(original); err != nil {
		t.Fatalf("caller mutated the stored binding: %v", err)
	}
	if err := controller.Configure(config); !errors.Is(err, ErrConfigurationConflict) {
		t.Fatalf("changed configuration bypassed conflict checks: %v", err)
	}
}

func TestForRuntimeReturnsIndependentSettings(t *testing.T) {
	config := mutableRunConfig()
	runtime := config.ForRuntime()
	runtime.MCPServers[0].URL = "https://changed.example.com/mcp"
	runtime.MCPServers[0].Headers["Authorization"] = "changed-header"
	runtime.Skills[0].SkillMD = "Changed instructions"
	if config.MCPServers[0].URL != "https://example.com/mcp" || config.MCPServers[0].Headers["Authorization"] != "private-header" || config.Skills[0].SkillMD != "Work instructions" {
		t.Fatal("runtime recipient can mutate the run's settings")
	}
}

func TestConfigurationJSONKeepsFlatRuntimeFields(t *testing.T) {
	config := mutableRunConfig()
	config.RestoreSession = true
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["organizationId"]) != `"org"` || string(fields["restoreSession"]) != "true" || fields["RuntimeConfig"] != nil || fields["repositories"] != nil {
		t.Fatalf("configuration wire format changed: %s", data)
	}
	var decoded configmodel.RunConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("flat runtime settings did not round trip: %v", err)
	}
}

func TestValidationExplainsInvalidSettingsWithoutValues(t *testing.T) {
	for _, test := range []struct {
		name, field, reason string
		change              func(*configmodel.RunConfig)
	}{
		{"required", "model.id", "is required", func(c *configmodel.RunConfig) { c.Model.ID = "" }},
		{"runtime", "runtime", "is unsupported", func(c *configmodel.RunConfig) { c.Runtime = "private-runtime" }},
		{"transport", "mcpServers.transport", "must be http or stdio", func(c *configmodel.RunConfig) { c.MCPServers[0].Transport = "" }},
		{"URL", "mcpServers", "must use HTTP or HTTPS URLs without user information", func(c *configmodel.RunConfig) { c.MCPServers[0].URL = "https://secret:password@example.com" }},
		{"headers", "mcpServers.headers", "must have nonempty names and no line breaks", func(c *configmodel.RunConfig) { c.MCPServers[0].Headers["Authorization"] = "secret\r\nvalue" }},
		{"skill ID", "skills", "must use lowercase alphanumeric IDs separated by single hyphens", func(c *configmodel.RunConfig) { c.Skills[0].ID = "INVALID" }},
		{"duplicate skills", "skills", "must have unique IDs", func(c *configmodel.RunConfig) { c.Skills = append(c.Skills, c.Skills[0]) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := mutableRunConfig()
			test.change(&config)
			for _, err := range []error{config.Validate(), config.ForRuntime().Validate()} {
				var validation *configmodel.ValidationError
				if !errors.As(err, &validation) || validation.Field != test.field || validation.Error() != test.field+" "+test.reason {
					t.Fatalf("unexpected validation: %v", err)
				}
				if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "password") {
					t.Fatal("validation exposed a rejected value")
				}
			}
		})
	}
}
