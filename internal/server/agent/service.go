package agent

import (
	"context"
	"fmt"
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	agentmodel "github.com/josemiguelmelo/orkest/internal/server/agent/model"
)

type AgentDefinitionUseCase struct {
	repository      AgentDefinitionRepository
	workspaces      WorkspaceAccess
	mcpServers      MCPServerAccess
	skills          SkillAccess
	profiles        ExecutionProfileValidator
	runtimeProfiles RuntimeProfileResolver
}

type (
	ExecutionProfileValidator interface {
		ValidateProfile(context.Context, string, string, agentmodel.Runtime) error
	}
	RuntimeProfileResolver interface {
		ResolveAgentProfile(context.Context, string, string) (agentmodel.Runtime, string, string, map[string]string, error)
	}
)

func NewAgentDefinitionUseCase(repository AgentDefinitionRepository, workspaces WorkspaceAccess, mcpServers MCPServerAccess, skills SkillAccess, profiles ExecutionProfileValidator, runtimeProfiles RuntimeProfileResolver) *AgentDefinitionUseCase {
	return &AgentDefinitionUseCase{repository: repository, workspaces: workspaces, mcpServers: mcpServers, skills: skills, profiles: profiles, runtimeProfiles: runtimeProfiles}
}

func (s *AgentDefinitionUseCase) applyRuntimeProfile(ctx context.Context, workspaceID string, d *agentmodel.AgentDefinition) error {
	if d.RuntimeProfileID == "" {
		return nil
	}
	if s.runtimeProfiles == nil {
		return modelerrors.ErrInvalid
	}
	runtime, provider, model, configuration, err := s.runtimeProfiles.ResolveAgentProfile(ctx, workspaceID, d.RuntimeProfileID)
	if err != nil {
		return err
	}
	configuration = coreruntime.NormalizeConfiguration(configuration)
	for key, value := range d.Configuration {
		if value != "" {
			configuration[key] = value
		}
	}
	d.Runtime, d.Provider, d.Model = runtime, provider, model
	d.Configuration = configuration
	return nil
}

func (s *AgentDefinitionUseCase) validateProfile(ctx context.Context, workspaceID string, d agentmodel.AgentDefinition) error {
	if d.ExecutionProfileID == "" {
		return nil
	}
	if s.profiles == nil {
		return modelerrors.ErrInvalid
	}
	return s.profiles.ValidateProfile(ctx, workspaceID, d.ExecutionProfileID, d.Runtime)
}

func (s *AgentDefinitionUseCase) List(ctx context.Context, userID, orgID, workspaceID string) ([]agentmodel.Agent, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.Agents(ctx, orgID, workspaceID)
}

func (s *AgentDefinitionUseCase) Get(ctx context.Context, userID, orgID, workspaceID, agentID string) (agentmodel.Agent, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return agentmodel.Agent{}, err
	}
	return s.repository.Agent(ctx, orgID, workspaceID, agentID)
}

func (s *AgentDefinitionUseCase) Create(ctx context.Context, userID, orgID, workspaceID string, definition agentmodel.AgentDefinition) (agentmodel.Agent, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return agentmodel.Agent{}, err
	}
	if err := s.applyRuntimeProfile(ctx, workspaceID, &definition); err != nil {
		return agentmodel.Agent{}, err
	}
	definition.Configuration = coreruntime.NormalizeConfiguration(definition.Configuration)
	if err := s.validateProfile(ctx, workspaceID, definition); err != nil {
		return agentmodel.Agent{}, err
	}
	if err := s.validateDefinition(&definition); err != nil {
		return agentmodel.Agent{}, err
	}
	if err := coreruntime.ValidateConfiguration(definition.Runtime, definition.Configuration); err != nil {
		return agentmodel.Agent{}, modelerrors.ErrInvalid
	}
	definition.HasConfiguration = len(definition.Configuration) > 0
	if err := s.validateMCPServers(ctx, orgID, workspaceID, definition.MCPServerIDs); err != nil {
		return agentmodel.Agent{}, err
	}
	if err := s.validateSkills(ctx, orgID, workspaceID, definition.SkillIDs); err != nil {
		return agentmodel.Agent{}, err
	}
	id, err := ids.NewUUID()
	if err != nil {
		return agentmodel.Agent{}, fmt.Errorf("generate agent ID: %w", err)
	}
	return s.repository.CreateAgent(ctx, id, workspaceID, definition)
}

func (s *AgentDefinitionUseCase) Update(ctx context.Context, userID, orgID, workspaceID, agentID string, definition agentmodel.AgentDefinition) (agentmodel.Agent, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return agentmodel.Agent{}, err
	}
	if definition.SkillIDs == nil {
		current, err := s.repository.Agent(ctx, orgID, workspaceID, agentID)
		if err != nil {
			return agentmodel.Agent{}, err
		}
		definition.SkillIDs = current.SkillIDs
	}
	current, currentErr := s.repository.Agent(ctx, orgID, workspaceID, agentID)
	if currentErr != nil {
		return agentmodel.Agent{}, currentErr
	}
	requestedConfiguration := coreruntime.NormalizeConfiguration(definition.Configuration)
	if definition.RuntimeProfileID != "" {
		definition.Configuration = requestedConfiguration
	} else {
		configuration := current.Configuration
		if configuration == nil {
			configuration = map[string]string{}
		}
		if provider, ok := coreruntime.ProviderFor(definition.Runtime); ok {
			allowed := make(map[string]bool)
			for _, option := range provider.ConfigurationSchema() {
				allowed[option.Name] = true
			}
			for key := range configuration {
				if !allowed[key] {
					delete(configuration, key)
				}
			}
		}
		for key, value := range requestedConfiguration {
			configuration[key] = value
		}
		definition.Configuration = configuration
	}
	if definition.MCPServerIDs == nil {
		current, err := s.repository.Agent(ctx, orgID, workspaceID, agentID)
		if err != nil {
			return agentmodel.Agent{}, err
		}
		definition.MCPServerIDs = current.MCPServerIDs
	}
	if err := s.applyRuntimeProfile(ctx, workspaceID, &definition); err != nil {
		return agentmodel.Agent{}, err
	}
	if err := s.validateProfile(ctx, workspaceID, definition); err != nil {
		return agentmodel.Agent{}, err
	}
	if err := s.validateDefinition(&definition); err != nil {
		return agentmodel.Agent{}, err
	}
	if err := coreruntime.ValidateConfiguration(definition.Runtime, definition.Configuration); err != nil {
		return agentmodel.Agent{}, modelerrors.ErrInvalid
	}
	definition.HasConfiguration = len(definition.Configuration) > 0
	if err := s.validateMCPServers(ctx, orgID, workspaceID, definition.MCPServerIDs); err != nil {
		return agentmodel.Agent{}, err
	}
	if err := s.validateSkills(ctx, orgID, workspaceID, definition.SkillIDs); err != nil {
		return agentmodel.Agent{}, err
	}
	return s.repository.UpdateAgent(ctx, orgID, workspaceID, agentID, definition)
}

func (s *AgentDefinitionUseCase) validateDefinition(definition *agentmodel.AgentDefinition) error {
	if err := validate(definition); err != nil {
		return err
	}
	if provider, ok := coreruntime.ProviderFor(definition.Runtime); ok {
		return provider.ValidateDefinition(coreruntime.Definition{ModelProvider: definition.Provider})
	}
	return nil
}

func (s *AgentDefinitionUseCase) validateSkills(ctx context.Context, orgID, workspaceID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if s.skills == nil {
		return modelerrors.ErrInvalid
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || seen[id] {
			return modelerrors.ErrInvalid
		}
		seen[id] = true
	}
	return s.skills.ValidateAgentSkills(ctx, orgID, workspaceID, ids)
}

func (s *AgentDefinitionUseCase) validateMCPServers(ctx context.Context, orgID, workspaceID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if s.mcpServers == nil {
		return modelerrors.ErrInvalid
	}
	ok, err := s.mcpServers.Exists(ctx, orgID, workspaceID, ids)
	if err != nil {
		return err
	}
	if !ok {
		return modelerrors.ErrInvalid
	}
	return nil
}

func (s *AgentDefinitionUseCase) Delete(ctx context.Context, userID, orgID, workspaceID, agentID string) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	return s.repository.DeleteAgent(ctx, orgID, workspaceID, agentID)
}

func (s *AgentDefinitionUseCase) authorize(ctx context.Context, userID, orgID, workspaceID string) error {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role == "viewer" {
		return modelerrors.ErrForbidden
	}
	return nil
}
