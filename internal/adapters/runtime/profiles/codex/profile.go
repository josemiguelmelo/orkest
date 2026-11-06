// Package codex defines the Codex ACP runtime profile.
package codex

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

var Version = coreruntime.Supported()[coreruntime.Codex]

type Profile struct{ Binary string }

func (p Profile) Descriptor() agentruntime.RuntimeDescriptor {
	binary := p.Binary
	if binary == "" {
		binary = "codex-acp"
	}
	return agentruntime.RuntimeDescriptor{
		Runtime: coreruntime.Codex, Version: Version, VersionOutputPrefix: "@agentclientprotocol/codex-acp ", Executable: binary,
		VersionArguments: []string{"--version"},
		ReservedEnvNames: []string{"HOME", "CODEX_HOME", "CODEX_PATH", "CODEX_CONFIG", "MODEL_PROVIDER", "APP_SERVER_LOGS", "NO_BROWSER", "INITIAL_AGENT_MODE", "DEFAULT_AUTH_REQUEST", "ORKEST_CODEX_AUTH_FILE"},
	}
}

func (Profile) BuildProvisioningPlan(settings agentruntime.ResolvedRuntimeSettings, paths agentruntime.ProvisioningPaths) (agentruntime.ProvisioningPlan, error) {
	if paths.RuntimeRoot == "" || paths.WorkDir == "" {
		return agentruntime.ProvisioningPlan{}, errors.New("runtime provisioning paths are required")
	}
	for _, skill := range settings.Skills {
		if !configmodel.ValidSkillID(skill.ID) {
			return agentruntime.ProvisioningPlan{}, errors.New("invalid skill ID")
		}
	}
	instructions := strings.TrimSpace(settings.Agent.Instructions)
	for _, skill := range settings.Skills {
		if strings.TrimSpace(skill.SkillMD) != "" {
			instructions += "\n\n## Skill: " + skill.ID + "\n\n" + strings.TrimSpace(skill.SkillMD)
		}
	}
	configuration := map[string]any{"model": settings.Model.ID, "developer_instructions": instructions}
	encodedConfig, err := json.Marshal(configuration)
	if err != nil {
		return agentruntime.ProvisioningPlan{}, errors.New("encode Codex configuration failed")
	}
	initialAgentMode := "workspace-write"
	if settings.Agent.PermissionPolicy == configmodel.PermissionPolicyAutonomous {
		initialAgentMode = "agent-full-access"
	}
	return agentruntime.ProvisioningPlan{
		Directories: []string{"data", "state", "cache", "home", "home/.codex", "state/codex/logs"},
		Environment: []string{
			"PATH=" + agentruntime.DefaultRuntimePath,
			"HOME=" + filepath.ToSlash(filepath.Join(paths.RuntimeRoot, "home")),
			"CODEX_HOME=" + filepath.ToSlash(filepath.Join(paths.RuntimeRoot, "home", ".codex")),
			"XDG_DATA_HOME=" + filepath.ToSlash(filepath.Join(paths.RuntimeRoot, "data")),
			"CODEX_CONFIG=" + string(encodedConfig),
			"MODEL_PROVIDER=openai",
			`DEFAULT_AUTH_REQUEST={"methodId":"api-key"}`,
			"APP_SERVER_LOGS=" + filepath.ToSlash(filepath.Join(paths.RuntimeRoot, "state", "codex", "logs")),
			"NO_BROWSER=1",
			"INITIAL_AGENT_MODE=" + initialAgentMode,
		},
	}, nil
}

func (Profile) NativeSessionMode(settings agentruntime.ResolvedRuntimeSettings) string {
	if settings.Agent.PermissionPolicy == configmodel.PermissionPolicyAutonomous {
		return "agent-full-access"
	}
	return ""
}

func (Profile) ProtocolSessionOptions(settings agentruntime.ResolvedRuntimeSettings, _ agentruntime.ProvisioningPaths) agentruntime.ProtocolSessionOptions {
	return agentruntime.ProtocolSessionOptions{
		Model:      settings.Model.ID,
		MCPServers: settings.MCPServers,
	}
}

var _ agentruntime.RuntimeProfile = Profile{}
