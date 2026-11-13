package workflowstageintegration

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
	"time"

	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/platform/secretbox"
	codemodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	stageintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
}

type CodingAccess interface {
	CodingCredential(context.Context, string, string) (codemodel.Credentials, error)
	ListIntegrations(context.Context, string, string, string) ([]codemodel.CodingIntegration, error)
}

type WorkflowStageIntegrationService struct {
	repo      WorkflowStageIntegrationRepository
	box       *secretbox.Box
	access    WorkspaceAccess
	coding    CodingAccess
	logger    *slog.Logger
	providers map[string]WorkflowStageIntegrationProvider
	now       func() time.Time
}

func NewWorkflowStageIntegrationService(repo WorkflowStageIntegrationRepository, box *secretbox.Box, access WorkspaceAccess, coding CodingAccess, logger *slog.Logger, providers ...WorkflowStageIntegrationProvider) *WorkflowStageIntegrationService {
	s := &WorkflowStageIntegrationService{repo: repo, box: box, access: access, coding: coding, logger: logger, now: func() time.Time { return time.Now().UTC() }, providers: map[string]WorkflowStageIntegrationProvider{}}
	for _, p := range providers {
		s.providers[p.Provider()] = p
	}
	return s
}

func (s *WorkflowStageIntegrationService) Providers() []stageintegrationmodel.ProviderInfo {
	result := make([]stageintegrationmodel.ProviderInfo, 0, len(s.providers))
	for _, p := range s.providers {
		result = append(result, stageintegrationmodel.ProviderInfo{Provider: p.Provider(), ConfigSchema: p.ConfigSchema(), Actions: p.Actions()})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Provider < result[j].Provider })
	return result
}

func (s *WorkflowStageIntegrationService) authorize(ctx context.Context, userID, orgID, workspaceID string, admin bool) error {
	w, err := s.access.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if admin && w.Role != "admin" && w.Role != "owner" {
		return problem.ErrForbidden
	}
	return nil
}

func (s *WorkflowStageIntegrationService) List(ctx context.Context, userID, orgID, workspaceID string) ([]stageintegrationmodel.Connection, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID, false); err != nil {
		return nil, err
	}
	return s.repo.ListConnections(ctx, workspaceID)
}

func (s *WorkflowStageIntegrationService) validateInput(ctx context.Context, userID, orgID, workspaceID string, in stageintegrationmodel.ConnectionInput) (stageintegrationmodel.ConnectionInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	p, ok := s.providers[in.Provider]
	if !ok || in.Name == "" || len(in.Name) > 200 || in.CodingIntegrationID == "" || len(in.RepositoryIDs) == 0 || len(in.RepositoryIDs) > 100 {
		return in, problem.ErrInvalid
	}
	if len(in.Config) == 0 {
		in.Config = json.RawMessage(`{}`)
	}
	if err := p.ValidateConfig(in.Config); err != nil {
		return in, err
	}
	seen := map[string]bool{}
	for _, id := range in.RepositoryIDs {
		if id == "" || seen[id] {
			return in, problem.ErrInvalid
		}
		seen[id] = true
	}
	integrations, err := s.coding.ListIntegrations(ctx, userID, orgID, workspaceID)
	if err != nil {
		return in, err
	}
	found := false
	for _, i := range integrations {
		if i.ID == in.CodingIntegrationID {
			if provider, ok := p.(interface{ CodingProvider() string }); ok && i.ProviderName != provider.CodingProvider() {
				return in, problem.ErrInvalid
			}
			found = true
		}
	}
	if !found {
		return in, problem.ErrInvalid
	}
	return in, nil
}

func (s *WorkflowStageIntegrationService) Create(ctx context.Context, userID, orgID, workspaceID string, in stageintegrationmodel.ConnectionInput) (stageintegrationmodel.Connection, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID, true); err != nil {
		return stageintegrationmodel.Connection{}, err
	}
	if s.box == nil {
		return stageintegrationmodel.Connection{}, secretbox.ErrKeyUnavailable
	}
	in, err := s.validateInput(ctx, userID, orgID, workspaceID, in)
	if err != nil {
		return stageintegrationmodel.Connection{}, err
	}
	if len(in.WebhookSecret) < 32 || len(in.WebhookSecret) > 4096 {
		return stageintegrationmodel.Connection{}, problem.ErrInvalid
	}
	encrypted, err := s.box.Seal(in.WebhookSecret)
	if err != nil {
		return stageintegrationmodel.Connection{}, err
	}
	id, err := ids.NewUUID()
	if err != nil {
		return stageintegrationmodel.Connection{}, err
	}
	return s.repo.CreateConnection(ctx, StoredConnection{Connection: stageintegrationmodel.Connection{ID: id, WorkspaceID: workspaceID, Provider: in.Provider, Name: in.Name, Enabled: in.Enabled, CodingIntegrationID: in.CodingIntegrationID, RepositoryIDs: in.RepositoryIDs, Config: in.Config, Version: 1, ConfiguredBy: userID}, EncryptedSecret: string(encrypted)})
}

func (s *WorkflowStageIntegrationService) Update(ctx context.Context, userID, orgID, workspaceID, id string, in stageintegrationmodel.ConnectionInput) (stageintegrationmodel.Connection, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID, true); err != nil {
		return stageintegrationmodel.Connection{}, err
	}
	if s.box == nil {
		return stageintegrationmodel.Connection{}, secretbox.ErrKeyUnavailable
	}
	old, err := s.repo.GetConnection(ctx, workspaceID, id)
	if err != nil {
		return stageintegrationmodel.Connection{}, err
	}
	if in.Provider != old.Connection.Provider || in.Version != old.Connection.Version {
		return stageintegrationmodel.Connection{}, problem.ErrConflict
	}
	in, err = s.validateInput(ctx, userID, orgID, workspaceID, in)
	if err != nil {
		return stageintegrationmodel.Connection{}, err
	}
	encrypted := old.EncryptedSecret
	if in.WebhookSecret != "" {
		if len(in.WebhookSecret) < 32 || len(in.WebhookSecret) > 4096 {
			return stageintegrationmodel.Connection{}, problem.ErrInvalid
		}
		sealed, e := s.box.Seal(in.WebhookSecret)
		if e != nil {
			return stageintegrationmodel.Connection{}, e
		}
		encrypted = string(sealed)
	}
	c := stageintegrationmodel.Connection{ID: id, WorkspaceID: workspaceID, Provider: in.Provider, Name: in.Name, Enabled: in.Enabled, CodingIntegrationID: in.CodingIntegrationID, RepositoryIDs: in.RepositoryIDs, Config: in.Config, Version: in.Version, ConfiguredBy: userID}
	return s.repo.UpdateConnection(ctx, StoredConnection{Connection: c, EncryptedSecret: encrypted})
}

func (s *WorkflowStageIntegrationService) Delete(ctx context.Context, userID, orgID, workspaceID, id string) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID, true); err != nil {
		return err
	}
	return s.repo.DeleteConnection(ctx, workspaceID, id)
}

func (s *WorkflowStageIntegrationService) Validate(ctx context.Context, userID, orgID, workspaceID, id string) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID, true); err != nil {
		return err
	}
	c, err := s.repo.GetConnection(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	credential, err := s.coding.CodingCredential(ctx, workspaceID, c.Connection.CodingIntegrationID)
	if err != nil {
		return err
	}
	p := s.providers[c.Connection.Provider]
	if p == nil {
		return problem.ErrInvalid
	}
	if validator, ok := p.(interface {
		ValidateConnection(context.Context, json.RawMessage, string) error
	}); ok {
		return validator.ValidateConnection(ctx, c.Connection.Config, credential.Token)
	}
	return p.ValidateConfig(c.Connection.Config)
}

func (s *WorkflowStageIntegrationService) Status(ctx context.Context, userID, orgID, workspaceID, connectionID string) ([]stageintegrationmodel.Status, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID, false); err != nil {
		return nil, err
	}
	return s.repo.ListStatus(ctx, workspaceID, connectionID)
}

func (s *WorkflowStageIntegrationService) Retry(ctx context.Context, userID, orgID, workspaceID, eventID string) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID, true); err != nil {
		return err
	}
	return s.repo.Retry(ctx, workspaceID, eventID)
}
