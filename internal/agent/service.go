package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
)

type Service struct {
	repository Repository
	workspaces WorkspaceAccess
	mcpServers MCPServerAccess
	skills     SkillAccess
	profiles   interface {
		ValidateProfile(context.Context, string, string, Runtime) error
	}
	runtimeProfiles interface {
		ResolveAgentProfile(context.Context, string, string) (Runtime, string, string, error)
	}
}

func NewService(repository Repository, workspaces WorkspaceAccess, mcpServers MCPServerAccess, skills SkillAccess) *Service {
	return &Service{repository: repository, workspaces: workspaces, mcpServers: mcpServers, skills: skills}
}

func (s *Service) SetProfiles(profiles interface {
	ValidateProfile(context.Context, string, string, Runtime) error
},
) {
	s.profiles = profiles
}

func (s *Service) SetRuntimeProfiles(profiles interface {
	ResolveAgentProfile(context.Context, string, string) (Runtime, string, string, error)
}) {
	s.runtimeProfiles = profiles
}

func (s *Service) applyRuntimeProfile(ctx context.Context, workspaceID string, d *AgentDefinition) error {
	if d.RuntimeProfileID == "" {
		return nil
	}
	if s.runtimeProfiles == nil {
		return modelerrors.ErrInvalid
	}
	runtime, provider, model, err := s.runtimeProfiles.ResolveAgentProfile(ctx, workspaceID, d.RuntimeProfileID)
	if err != nil {
		return err
	}
	d.Runtime, d.Provider, d.Model = runtime, provider, model
	return nil
}

func (s *Service) validateProfile(ctx context.Context, workspaceID string, d AgentDefinition) error {
	if d.ExecutionProfileID == "" {
		return nil
	}
	if s.profiles == nil {
		return modelerrors.ErrInvalid
	}
	return s.profiles.ValidateProfile(ctx, workspaceID, d.ExecutionProfileID, d.Runtime)
}

func (s *Service) List(ctx context.Context, userID, orgID, workspaceID string) ([]Agent, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.Agents(ctx, orgID, workspaceID)
}

func (s *Service) Get(ctx context.Context, userID, orgID, workspaceID, agentID string) (Agent, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return Agent{}, err
	}
	return s.repository.Agent(ctx, orgID, workspaceID, agentID)
}

func (s *Service) Create(ctx context.Context, userID, orgID, workspaceID string, definition AgentDefinition) (Agent, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Agent{}, err
	}
	if err := s.applyRuntimeProfile(ctx, workspaceID, &definition); err != nil {
		return Agent{}, err
	}
	if err := s.validateProfile(ctx, workspaceID, definition); err != nil {
		return Agent{}, err
	}
	if err := s.validateDefinition(&definition); err != nil {
		return Agent{}, err
	}
	if err := s.validateMCPServers(ctx, orgID, workspaceID, definition.MCPServerIDs); err != nil {
		return Agent{}, err
	}
	if err := s.validateSkills(ctx, orgID, workspaceID, definition.SkillIDs); err != nil {
		return Agent{}, err
	}
	id, err := ids.NewUUID()
	if err != nil {
		return Agent{}, fmt.Errorf("generate agent ID: %w", err)
	}
	return s.repository.CreateAgent(ctx, id, workspaceID, definition)
}

func (s *Service) Update(ctx context.Context, userID, orgID, workspaceID, agentID string, definition AgentDefinition) (Agent, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Agent{}, err
	}
	if definition.SkillIDs == nil {
		current, err := s.repository.Agent(ctx, orgID, workspaceID, agentID)
		if err != nil {
			return Agent{}, err
		}
		definition.SkillIDs = current.SkillIDs
	}
	if definition.MCPServerIDs == nil {
		current, err := s.repository.Agent(ctx, orgID, workspaceID, agentID)
		if err != nil {
			return Agent{}, err
		}
		definition.MCPServerIDs = current.MCPServerIDs
	}
	if err := s.applyRuntimeProfile(ctx, workspaceID, &definition); err != nil {
		return Agent{}, err
	}
	if err := s.validateProfile(ctx, workspaceID, definition); err != nil {
		return Agent{}, err
	}
	if err := s.validateDefinition(&definition); err != nil {
		return Agent{}, err
	}
	if err := s.validateMCPServers(ctx, orgID, workspaceID, definition.MCPServerIDs); err != nil {
		return Agent{}, err
	}
	if err := s.validateSkills(ctx, orgID, workspaceID, definition.SkillIDs); err != nil {
		return Agent{}, err
	}
	return s.repository.UpdateAgent(ctx, orgID, workspaceID, agentID, definition)
}

func (s *Service) validateDefinition(definition *AgentDefinition) error {
	if err := validate(definition); err != nil {
		return err
	}
	if provider, ok := coreruntime.ProviderFor(definition.Runtime); ok {
		return provider.ValidateDefinition(coreruntime.Definition{ModelProvider: definition.Provider})
	}
	return nil
}

func (s *Service) validateSkills(ctx context.Context, orgID, workspaceID string, ids []string) error {
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

func (s *Service) validateMCPServers(ctx context.Context, orgID, workspaceID string, ids []string) error {
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

func (s *Service) Delete(ctx context.Context, userID, orgID, workspaceID, agentID string) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	return s.repository.DeleteAgent(ctx, orgID, workspaceID, agentID)
}

func (s *Service) authorize(ctx context.Context, userID, orgID, workspaceID string) error {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role == "viewer" {
		return modelerrors.ErrForbidden
	}
	return nil
}

func validate(definition *AgentDefinition) error {
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
	seen := make(map[string]bool, len(definition.MCPServerIDs))
	for i, id := range definition.MCPServerIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return modelerrors.ErrInvalid
		}
		seen[id] = true
		definition.MCPServerIDs[i] = id
	}
	seenSkills := make(map[string]bool, len(definition.SkillIDs))
	for i, id := range definition.SkillIDs {
		id = strings.TrimSpace(id)
		if id == "" || seenSkills[id] {
			return modelerrors.ErrInvalid
		}
		seenSkills[id] = true
		definition.SkillIDs[i] = id
	}
	if definition.Name == "" || len(definition.Name) > 200 || definition.Provider == "" || len(definition.Provider) > 200 || definition.Model == "" || len(definition.Model) > 200 || len(definition.Description) > 2000 || len(definition.SystemPrompt) > 100000 || !definition.Runtime.Available() {
		return modelerrors.ErrInvalid
	}
	return nil
}
