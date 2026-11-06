package agent

import (
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	agentmodel "github.com/josemiguelmelo/orkest/internal/server/agent/model"
)

func validate(definition *agentmodel.AgentDefinition) error {
	if definition.PermissionPolicy == "" {
		definition.PermissionPolicy = agentmodel.PermissionPolicyAsk
	}
	if definition.MCPServerIDs == nil {
		definition.MCPServerIDs = []string{}
	}
	if definition.SkillIDs == nil {
		definition.SkillIDs = []string{}
	}
	definition.Name = strings.TrimSpace(definition.Name)
	definition.Provider = strings.TrimSpace(definition.Provider)
	definition.Model = strings.TrimSpace(definition.Model)
	definition.Description = strings.TrimSpace(definition.Description)
	if err := normalizeSelectedIDs(definition.MCPServerIDs); err != nil {
		return err
	}
	if err := normalizeSelectedIDs(definition.SkillIDs); err != nil {
		return err
	}

	if definition.Name == "" || len(definition.Name) > 200 || definition.Provider == "" || len(definition.Provider) > 200 || len(definition.Model) > 200 || len(definition.Description) > 2000 || len(definition.SystemPrompt) > 100000 || !definition.Runtime.Available() || (definition.PermissionPolicy != agentmodel.PermissionPolicyAsk && definition.PermissionPolicy != agentmodel.PermissionPolicyAutonomous) {
		return modelerrors.ErrInvalid
	}
	return nil
}

func normalizeSelectedIDs(values []string) error {
	seen := make(map[string]bool, len(values))
	for i, id := range values {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return modelerrors.ErrInvalid
		}
		seen[id] = true
		values[i] = id
	}
	return nil
}
