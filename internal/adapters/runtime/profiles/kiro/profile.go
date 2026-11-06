// Package kiro defines Kiro CLI's ACP runtime profile.
package kiro

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

var Version = coreruntime.Supported()[coreruntime.Kiro]

type Profile struct{ Binary string }

func (p Profile) Descriptor() agentruntime.RuntimeDescriptor {
	binary := p.Binary
	if binary == "" {
		binary = "kiro-cli"
	}
	return agentruntime.RuntimeDescriptor{
		Runtime: coreruntime.Kiro, Version: Version, VersionOutputPrefix: "kiro-cli ", Executable: binary,
		Arguments: []string{"acp", "--agent-engine=v3", "--agent", "orkest"}, VersionArguments: []string{"--version"},
		ReservedEnvNames: []string{"HOME", "KIRO_HOME", "KIRO_SKIP_BINARY_PINNING"},
	}
}

type agentConfig struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Prompt      string            `json:"prompt"`
	Model       string            `json:"model"`
	Tools       []string          `json:"tools"`
	Permissions permissionsConfig `json:"permissions"`
}

type permissionsConfig struct {
	Rules []permissionRule `json:"rules,omitempty"`
}

type permissionRule struct {
	Capability string `json:"capability"`
	Effect     string `json:"effect"`
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
		if !configmodel.ValidSkillID(skill.ID) {
			return agentruntime.ProvisioningPlan{}, errors.New("invalid skill ID")
		}
	}
	if strings.TrimSpace(settings.Model.ID) == "" {
		return agentruntime.ProvisioningPlan{}, errors.New("kiro model is required")
	}
	prompt := strings.TrimSpace(settings.Agent.Instructions)
	for _, skill := range settings.Skills {
		prompt += "\n\n## Skill: " + skill.ID + "\n\n" + strings.TrimSpace(skill.SkillMD)
	}
	profile := agentConfig{Name: "orkest", Description: "Orkest task agent", Prompt: prompt, Model: settings.Model.ID, Tools: []string{"*"}}
	if settings.Agent.PermissionPolicy == configmodel.PermissionPolicyAutonomous {
		profile.Permissions.Rules = []permissionRule{{Capability: "all", Effect: "allow"}}
	}
	contents, err := json.Marshal(profile)
	if err != nil {
		return agentruntime.ProvisioningPlan{}, errors.New("encode Kiro agent configuration failed")
	}
	kiroHome := filepath.ToSlash(filepath.Join(paths.RuntimeRoot, "home", ".kiro"))
	profilePath := filepath.ToSlash(filepath.Join("home", ".kiro", "agents", "orkest.json"))
	return agentruntime.ProvisioningPlan{
		Directories: []string{filepath.ToSlash(filepath.Join("home", ".kiro", "agents"))},
		Files:       map[string][]byte{profilePath: contents},
		Environment: []string{
			"PATH=" + agentruntime.DefaultRuntimePath,
			"HOME=" + filepath.ToSlash(filepath.Join(paths.RuntimeRoot, "home")),
			"KIRO_HOME=" + kiroHome,
		},
	}, nil
}

func (Profile) NativeSessionMode(agentruntime.ResolvedRuntimeSettings) string { return "" }

func (Profile) ProtocolSessionOptions(settings agentruntime.ResolvedRuntimeSettings, _ agentruntime.ProvisioningPaths) agentruntime.ProtocolSessionOptions {
	return agentruntime.ProtocolSessionOptions{Model: settings.Model.ID, MCPServers: settings.MCPServers}
}

var _ agentruntime.RuntimeProfile = Profile{}
