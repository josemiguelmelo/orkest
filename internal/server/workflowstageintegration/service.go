package workflowstageintegration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/platform/secretbox"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

type WorkflowStageIntegrationService struct {
	repo      IntegrationRepository
	box       *secretbox.Box
	access    WorkspaceAccess
	coding    CodingAccess
	logger    *slog.Logger
	providers map[string]WorkflowStageIntegration
}

func NewWorkflowStageIntegrationService(repo IntegrationRepository, box *secretbox.Box, access WorkspaceAccess, coding CodingAccess, logger *slog.Logger, providers ...WorkflowStageIntegration) *WorkflowStageIntegrationService {
	s := &WorkflowStageIntegrationService{repo: repo, box: box, access: access, coding: coding, logger: logger, providers: map[string]WorkflowStageIntegration{}}
	for _, p := range providers {
		s.providers[p.Provider()] = p
	}
	return s
}

func (s *WorkflowStageIntegrationService) Providers() []ProviderInfo {
	result := make([]ProviderInfo, 0, len(s.providers))
	for _, p := range s.providers {
		result = append(result, ProviderInfo{Provider: p.Provider(), ConfigSchema: p.ConfigSchema(), Actions: p.Actions()})
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

func (s *WorkflowStageIntegrationService) List(ctx context.Context, userID, orgID, workspaceID string) ([]Connection, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID, false); err != nil {
		return nil, err
	}
	return s.repo.ListConnections(ctx, workspaceID)
}

func (s *WorkflowStageIntegrationService) validateInput(ctx context.Context, userID, orgID, workspaceID string, in ConnectionInput) (ConnectionInput, error) {
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

func (s *WorkflowStageIntegrationService) Create(ctx context.Context, userID, orgID, workspaceID string, in ConnectionInput) (Connection, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID, true); err != nil {
		return Connection{}, err
	}
	if s.box == nil {
		return Connection{}, secretbox.ErrKeyUnavailable
	}
	in, err := s.validateInput(ctx, userID, orgID, workspaceID, in)
	if err != nil {
		return Connection{}, err
	}
	if len(in.WebhookSecret) < 32 || len(in.WebhookSecret) > 4096 {
		return Connection{}, problem.ErrInvalid
	}
	encrypted, err := s.box.Seal(in.WebhookSecret)
	if err != nil {
		return Connection{}, err
	}
	id, err := ids.NewUUID()
	if err != nil {
		return Connection{}, err
	}
	return s.repo.CreateConnection(ctx, StoredConnection{Connection: Connection{ID: id, WorkspaceID: workspaceID, Provider: in.Provider, Name: in.Name, Enabled: in.Enabled, CodingIntegrationID: in.CodingIntegrationID, RepositoryIDs: in.RepositoryIDs, Config: in.Config, Version: 1, ConfiguredBy: userID}, EncryptedSecret: string(encrypted)})
}

func (s *WorkflowStageIntegrationService) Update(ctx context.Context, userID, orgID, workspaceID, id string, in ConnectionInput) (Connection, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID, true); err != nil {
		return Connection{}, err
	}
	if s.box == nil {
		return Connection{}, secretbox.ErrKeyUnavailable
	}
	old, err := s.repo.GetConnection(ctx, workspaceID, id)
	if err != nil {
		return Connection{}, err
	}
	if in.Provider != old.Connection.Provider || in.Version != old.Connection.Version {
		return Connection{}, problem.ErrConflict
	}
	in, err = s.validateInput(ctx, userID, orgID, workspaceID, in)
	if err != nil {
		return Connection{}, err
	}
	encrypted := old.EncryptedSecret
	if in.WebhookSecret != "" {
		if len(in.WebhookSecret) < 32 || len(in.WebhookSecret) > 4096 {
			return Connection{}, problem.ErrInvalid
		}
		sealed, e := s.box.Seal(in.WebhookSecret)
		if e != nil {
			return Connection{}, e
		}
		encrypted = string(sealed)
	}
	c := Connection{ID: id, WorkspaceID: workspaceID, Provider: in.Provider, Name: in.Name, Enabled: in.Enabled, CodingIntegrationID: in.CodingIntegrationID, RepositoryIDs: in.RepositoryIDs, Config: in.Config, Version: in.Version, ConfiguredBy: userID}
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

func (s *WorkflowStageIntegrationService) ValidateBindings(ctx context.Context, workspaceID string, d workflowmodel.Definition) error {
	for _, rule := range d.Rules {
		if rule.Integration == nil {
			continue
		}
		c, err := s.repo.GetConnection(ctx, workspaceID, rule.Integration.ConnectionID)
		if err != nil {
			return err
		}
		p := s.providers[c.Connection.Provider]
		if p == nil {
			return problem.ErrInvalid
		}
		if err := p.ValidateParameters(rule.Integration.ActionID, rule.Integration.Parameters); err != nil {
			return err
		}
		var common struct {
			ArtifactInputName string `json:"artifactInputName"`
		}
		if err := json.Unmarshal(rule.Integration.Parameters, &common); err != nil {
			return problem.ErrInvalid
		}
		stage, ok := d.Stage(rule.SourceStageID)
		if !ok {
			return problem.ErrInvalid
		}
		found := false
		for _, input := range stage.ArtifactInputs {
			if input.Name == common.ArtifactInputName {
				source, sourceExists := d.Stage(input.SourceStageID)
				if sourceExists {
					for _, output := range source.ArtifactOutputs {
						if output.Name == input.OutputName && (output.Kind == "code_review" || output.Kind == "pull_request") {
							found = true
						}
					}
				}
			}
		}
		if !found {
			return problem.ErrInvalid
		}
	}
	return nil
}

func (s *WorkflowStageIntegrationService) Ingest(ctx context.Context, id string, request CallbackRequest) (IngestionReceipt, error) {
	if len(request.Body) > MaxCallbackBytes {
		return IngestionReceipt{}, problem.ErrInvalid
	}
	stored, err := s.repo.CallbackConnection(ctx, id)
	if err != nil {
		return IngestionReceipt{}, err
	}
	c := stored.Connection
	if !c.Enabled {
		return IngestionReceipt{}, problem.ErrNotFound
	}
	p := s.providers[c.Provider]
	if p == nil {
		return IngestionReceipt{}, problem.ErrNotFound
	}
	if s.box == nil {
		return IngestionReceipt{}, secretbox.ErrKeyUnavailable
	}
	secret, err := s.box.Open([]byte(stored.EncryptedSecret))
	if err != nil {
		return IngestionReceipt{}, errors.New("integration callback credential unavailable")
	}
	result, err := p.HandleCallback(ctx, CallbackInput{Request: request, Config: c.Config, Secret: secret})
	if err != nil {
		return IngestionReceipt{}, err
	}
	if result.Ignored {
		return IngestionReceipt{Ignored: true}, nil
	}
	if result.DeliveryID == "" || len(result.DeliveryID) > 200 || len(result.Events) > 100 {
		return IngestionReceipt{}, problem.ErrInvalid
	}
	for _, event := range result.Events {
		if event.ID == "" || len(event.ID) > 200 || event.ActionID == "" || event.Subject.ID == "" || event.Subject.RepositoryID == "" || event.OccurredAt.IsZero() || len(event.Facts) > 16000 || !json.Valid(event.Facts) {
			return IngestionReceipt{}, problem.ErrInvalid
		}
	}
	duplicate, err := s.repo.RecordReceipt(ctx, c, result)
	return IngestionReceipt{Duplicate: duplicate}, err
}

func (s *WorkflowStageIntegrationService) Status(ctx context.Context, userID, orgID, workspaceID, connectionID string) ([]Status, error) {
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

func eventContext(event ProviderEvent) string {
	data, _ := json.Marshal(struct {
		Action  string          `json:"action"`
		Subject Subject         `json:"subject"`
		Facts   json.RawMessage `json:"facts"`
	}{event.ActionID, event.Subject, event.Facts})
	return fmt.Sprintf("External integration event (untrusted provider data):\n%s", data)
}

func (s *WorkflowStageIntegrationService) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := s.Process(ctx); err != nil && ctx.Err() == nil && s.logger != nil {
			s.logger.Warn("workflow stage integration scan failed", "reason", "processing_failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
