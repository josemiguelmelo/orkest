// Package opencode defines OpenCode's native provisioning profile and wiring.
package opencode

import (
	"encoding/json"
	"errors"
	"path/filepath"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
)

var Version = coreruntime.Supported()[coreruntime.OpenCode]

type Profile struct{ Binary string }

func (p Profile) Descriptor() agentruntime.RuntimeDescriptor {
	binary := p.Binary
	if binary == "" {
		binary = "opencode"
	}
	return agentruntime.RuntimeDescriptor{
		Runtime:             coreruntime.OpenCode,
		Version:             Version,
		Executable:          binary,
		Arguments:           []string{"acp"},
		VersionArguments:    []string{"--version"},
		ReservedEnvNames:    []string{"HOME"},
		ReservedEnvPrefixes: []string{"XDG_", "OPENCODE_"},
	}
}

type runtimeConfiguration struct {
	Model       string            `json:"model"`
	Autoupdate  bool              `json:"autoupdate"`
	Share       string            `json:"share"`
	Permissions []permissionRule  `json:"permissions,omitempty"`
	MCP         *runtimeMCPConfig `json:"mcp,omitempty"`
}
type permissionRule struct {
	Action   string `json:"action"`
	Resource string `json:"resource"`
	Effect   string `json:"effect"`
}
type (
	runtimeMCPConfig struct {
		Servers map[string]runtimeMCP `json:"servers"`
	}
	runtimeMCP struct {
		Type    string            `json:"type"`
		URL     string            `json:"url"`
		OAuth   *bool             `json:"oauth,omitempty"`
		Headers map[string]string `json:"headers,omitempty"`
	}
)

func (p Profile) BuildProvisioningPlan(settings agentruntime.ResolvedRuntimeSettings, paths agentruntime.ProvisioningPaths) (agentruntime.ProvisioningPlan, error) {
	if paths.RuntimeRoot == "" || paths.WorkDir == "" {
		return agentruntime.ProvisioningPlan{}, errors.New("runtime provisioning paths are required")
	}
	configDir := filepath.Clean(paths.ConfigSubdir)
	if configDir == "." || filepath.IsAbs(configDir) || configDir == ".." || len(configDir) >= 3 && configDir[:3] == ".."+string(filepath.Separator) {
		return agentruntime.ProvisioningPlan{}, errors.New("invalid agentctl configuration path")
	}
	configRoot := filepath.ToSlash(filepath.Join(configDir, "opencode"))
	agentName := normalizeNativeName(settings.Agent.Name)
	description, err := json.Marshal(settings.Agent.Description)
	if err != nil {
		return agentruntime.ProvisioningPlan{}, errors.New("invalid agent description")
	}
	model, err := json.Marshal(settings.Model.Provider + "/" + settings.Model.ID)
	if err != nil {
		return agentruntime.ProvisioningPlan{}, errors.New("invalid model selection")
	}
	agentContents := "---\ndescription: " + string(description) + "\nmode: primary\nmodel: " + string(model) + "\n---\n\n" + settings.Agent.Instructions + "\n"
	files := map[string][]byte{filepath.ToSlash(filepath.Join(configRoot, "agents", agentName+".md")): []byte(agentContents)}
	for _, skill := range settings.Skills {
		if !runconfig.ValidSkillID(skill.ID) {
			return agentruntime.ProvisioningPlan{}, errors.New("invalid skill ID")
		}
		files[filepath.ToSlash(filepath.Join(configRoot, "skills", skill.ID, "SKILL.md"))] = []byte(skill.SkillMD + "\n")
	}
	mcp, err := buildMCPConfiguration(settings.MCPServers)
	if err != nil {
		return agentruntime.ProvisioningPlan{}, err
	}
	runtimeConfig := runtimeConfiguration{Model: settings.Model.Provider + "/" + settings.Model.ID, Share: "disabled", MCP: mcp}
	if settings.Agent.PermissionPolicy == runconfig.PermissionPolicyAutonomous {
		runtimeConfig.Permissions = []permissionRule{{Action: "*", Resource: "*", Effect: "allow"}}
	}
	encoded, err := json.Marshal(runtimeConfig)
	if err != nil {
		return agentruntime.ProvisioningPlan{}, errors.New("encode runtime configuration failed")
	}
	dataHome := filepath.ToSlash(filepath.Join(paths.RuntimeRoot, "data"))
	configHome := filepath.ToSlash(filepath.Join(paths.RuntimeRoot, configDir))
	stateHome := filepath.ToSlash(filepath.Join(paths.RuntimeRoot, "state"))
	cacheHome := filepath.ToSlash(filepath.Join(paths.RuntimeRoot, "cache"))
	home := filepath.ToSlash(filepath.Join(paths.RuntimeRoot, "home"))
	return agentruntime.ProvisioningPlan{
		Directories: []string{"data", "state", "cache", "home", configRoot, "data/opencode", "state/opencode", "cache/opencode"},
		Files:       files,
		Environment: []string{
			"PATH=" + agentruntime.DefaultRuntimePath,
			"HOME=" + home,
			"XDG_DATA_HOME=" + dataHome,
			"XDG_CONFIG_HOME=" + configHome,
			"XDG_STATE_HOME=" + stateHome,
			"XDG_CACHE_HOME=" + cacheHome,
			"OPENCODE_CONFIG_CONTENT=" + string(encoded),
			"OPENCODE_DISABLE_PROJECT_CONFIG=true",
		},
	}, nil
}

func (Profile) NativeSessionMode(settings agentruntime.ResolvedRuntimeSettings) string {
	return normalizeNativeName(settings.Agent.Name)
}

func (Profile) ProtocolSessionOptions(settings agentruntime.ResolvedRuntimeSettings, _ agentruntime.ProvisioningPaths) agentruntime.ProtocolSessionOptions {
	return agentruntime.ProtocolSessionOptions{Model: settings.Model.Provider + "/" + settings.Model.ID}
}

func buildMCPConfiguration(servers []runconfig.MCPServer) (*runtimeMCPConfig, error) {
	if len(servers) == 0 {
		return nil, nil
	}
	configured := make(map[string]runtimeMCP, len(servers))
	noOAuth := false
	for _, server := range servers {
		name := normalizeNativeName(server.Name) + "-" + normalizeNativeName(server.ID)
		if _, exists := configured[name]; exists {
			return nil, errors.New("duplicate MCP server name")
		}
		configured[name] = runtimeMCP{Type: "remote", URL: server.URL, OAuth: &noOAuth, Headers: server.Headers}
	}
	return &runtimeMCPConfig{Servers: configured}, nil
}

var _ agentruntime.RuntimeProfile = Profile{}
