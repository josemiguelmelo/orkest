// Package githubcopilot defines GitHub Copilot CLI's ACP runtime profile.
package githubcopilot

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/agentctl/config"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

var Version = coreruntime.Supported()[coreruntime.GitHubCopilot]

type Profile struct{ Binary string }

func (p Profile) Descriptor() agentruntime.RuntimeDescriptor {
	binary := p.Binary
	if binary == "" {
		binary = "copilot"
	}
	return agentruntime.RuntimeDescriptor{
		Runtime: coreruntime.GitHubCopilot, Version: Version, VersionOutputPrefix: "GitHub Copilot CLI ", VersionOutputSuffix: ".", Executable: binary,
		Arguments: []string{"--acp", "--stdio"}, VersionArguments: []string{"--version"},
		ReservedEnvNames: []string{"HOME", "COPILOT_HOME", "COPILOT_MODEL", "COPILOT_AUTO_UPDATE", "COPILOT_ALLOW_ALL"},
	}
}

func (Profile) BuildProvisioningPlan(settings agentruntime.ResolvedRuntimeSettings, paths agentruntime.ProvisioningPaths) (agentruntime.ProvisioningPlan, error) {
	if paths.RuntimeRoot == "" || paths.WorkDir == "" {
		return agentruntime.ProvisioningPlan{}, errors.New("runtime provisioning paths are required")
	}
	configDir := filepath.Clean(paths.ConfigSubdir)
	if configDir == "." || filepath.IsAbs(configDir) || configDir == ".." || strings.HasPrefix(configDir, ".."+string(filepath.Separator)) {
		return agentruntime.ProvisioningPlan{}, errors.New("invalid agentctl configuration path")
	}
	for _, skill := range settings.Skills {
		if !config.ValidSkillID(skill.ID) {
			return agentruntime.ProvisioningPlan{}, errors.New("invalid skill ID")
		}
	}
	configRoot := filepath.ToSlash(filepath.Join(paths.RuntimeRoot, configDir, "copilot"))
	files := map[string][]byte{
		filepath.ToSlash(filepath.Join(configDir, "copilot", "copilot-instructions.md")): []byte(strings.TrimSpace(settings.Agent.Instructions) + "\n"),
	}
	for _, skill := range settings.Skills {
		files[filepath.ToSlash(filepath.Join(configDir, "copilot", "skills", skill.ID, "SKILL.md"))] = []byte(skill.SkillMD + "\n")
	}
	if settings.Model.ID == "" {
		return agentruntime.ProvisioningPlan{}, errors.New("copilot model is required")
	}
	environment := []string{
		"PATH=" + agentruntime.DefaultRuntimePath,
		"HOME=" + filepath.ToSlash(filepath.Join(paths.RuntimeRoot, "home")),
		"COPILOT_HOME=" + configRoot,
		"COPILOT_MODEL=" + settings.Model.ID,
		"COPILOT_AUTO_UPDATE=false",
	}
	if settings.Agent.PermissionPolicy == config.PermissionPolicyAutonomous {
		environment = append(environment, "COPILOT_ALLOW_ALL=true")
	}
	return agentruntime.ProvisioningPlan{
		Directories: []string{"home", filepath.ToSlash(filepath.Join(configDir, "copilot")), filepath.ToSlash(filepath.Join(configDir, "copilot", "skills"))},
		Files:       files, Environment: environment,
	}, nil
}

func (Profile) NativeSessionMode(agentruntime.ResolvedRuntimeSettings) string { return "" }

func (Profile) ProtocolSessionOptions(settings agentruntime.ResolvedRuntimeSettings, _ agentruntime.ProvisioningPaths) agentruntime.ProtocolSessionOptions {
	return agentruntime.ProtocolSessionOptions{Model: settings.Model.ID, MCPServers: settings.MCPServers}
}

var _ agentruntime.RuntimeProfile = Profile{}
