// Package claudecode defines the Claude Code ACP runtime profile.
package claudecode

import (
	"errors"
	"path/filepath"
	"strings"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
)

var Version = coreruntime.Supported()[coreruntime.ClaudeCode]

type Profile struct{ Binary string }

func (p Profile) Descriptor() agentruntime.RuntimeDescriptor {
	binary := p.Binary
	if binary == "" {
		binary = "claude-agent-acp"
	}
	return agentruntime.RuntimeDescriptor{
		Runtime: coreruntime.ClaudeCode, Version: Version, Executable: binary,
		VersionArguments:    []string{"--version"},
		ReservedEnvNames:    []string{"HOME", "CLAUDE_CONFIG_DIR", "CLAUDE_AGENT_ACP_PROTOCOL"},
		ReservedEnvPrefixes: []string{"CLAUDE_"},
	}
}

func (p Profile) BuildProvisioningPlan(_ agentruntime.ResolvedRuntimeSettings, paths agentruntime.ProvisioningPaths) (agentruntime.ProvisioningPlan, error) {
	if paths.RuntimeRoot == "" || paths.WorkDir == "" {
		return agentruntime.ProvisioningPlan{}, errors.New("runtime provisioning paths are required")
	}
	configDir := filepath.Clean(paths.ConfigSubdir)
	if configDir == "." || filepath.IsAbs(configDir) || configDir == ".." || strings.HasPrefix(configDir, ".."+string(filepath.Separator)) {
		return agentruntime.ProvisioningPlan{}, errors.New("invalid agentctl configuration path")
	}
	return agentruntime.ProvisioningPlan{
		Directories: []string{"data", "state", "cache", "home", filepath.ToSlash(filepath.Join(configDir, "claude"))},
		Environment: []string{
			"PATH=" + agentruntime.DefaultRuntimePath,
			"HOME=" + filepath.ToSlash(filepath.Join(paths.RuntimeRoot, "home")),
			"CLAUDE_CONFIG_DIR=" + filepath.ToSlash(filepath.Join(paths.RuntimeRoot, configDir, "claude")),
			"CLAUDE_AGENT_ACP_PROTOCOL=v1",
		},
	}, nil
}

func (Profile) NativeSessionMode(settings agentruntime.ResolvedRuntimeSettings) string {
	if settings.Agent.PermissionPolicy == runconfig.PermissionPolicyAutonomous {
		return "bypassPermissions"
	}
	return ""
}

func (Profile) ProtocolSessionOptions(settings agentruntime.ResolvedRuntimeSettings, _ agentruntime.ProvisioningPaths) agentruntime.ProtocolSessionOptions {
	instructions := strings.TrimSpace(settings.Agent.Instructions)
	for _, skill := range settings.Skills {
		if runconfig.ValidSkillID(skill.ID) && strings.TrimSpace(skill.SkillMD) != "" {
			instructions += "\n\n## Skill: " + skill.ID + "\n\n" + strings.TrimSpace(skill.SkillMD)
		}
	}
	allowSkipPermissions := settings.Agent.PermissionPolicy == runconfig.PermissionPolicyAutonomous
	return agentruntime.ProtocolSessionOptions{
		Metadata: map[string]any{
			"systemPrompt": instructions,
			"claudeCode": map[string]any{
				"options": map[string]any{
					"model":                           settings.Model.ID,
					"allowDangerouslySkipPermissions": allowSkipPermissions,
				},
			},
		},
		MCPServers: settings.MCPServers,
	}
}

var _ agentruntime.RuntimeProfile = Profile{}
