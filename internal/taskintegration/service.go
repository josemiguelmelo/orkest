package taskintegration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/josemiguelmelo/orkest/internal/core/secretbox"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type TaskIntegrationService struct {
	repo          TaskIntegrationRepository
	box           *secretbox.Box
	providers     map[TaskIntegrationSourceProvider]SourceIntegrationProvider
	workspaces    WorkspaceAccess
	organizations OrganizationAccess
	importer      ImportedTaskRepository
	mu            sync.Mutex
	inFlight      map[string]bool
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error)
}
type OrganizationAccess interface {
	RoleForUser(context.Context, string, string) (string, error)
}

func NewTaskIntegrationService(repo TaskIntegrationRepository, box *secretbox.Box, workspaces WorkspaceAccess, organizations OrganizationAccess, importer ImportedTaskRepository, providers ...SourceIntegrationProvider) *TaskIntegrationService {
	s := &TaskIntegrationService{repo: repo, box: box, workspaces: workspaces, organizations: organizations, importer: importer, providers: map[TaskIntegrationSourceProvider]SourceIntegrationProvider{}, inFlight: map[string]bool{}}
	for _, p := range providers {
		s.providers[p.Provider()] = p
	}
	return s
}

func (s *TaskIntegrationService) List(ctx context.Context, userID, orgID, workspaceID string) ([]TaskIntegration, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, workspaceID)
}

func (s *TaskIntegrationService) Providers() []SourceIntegrationProviderInfo {
	providers := make([]SourceIntegrationProviderInfo, 0, len(s.providers))
	for id, provider := range s.providers {
		providers = append(providers, SourceIntegrationProviderInfo{
			Provider:     id,
			ConfigSchema: append(json.RawMessage(nil), provider.ConfigSchema()...),
		})
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].Provider < providers[j].Provider })
	return providers
}

func (s *TaskIntegrationService) Create(ctx context.Context, userID, orgID, workspaceID string, in TaskIntegrationInput) (TaskIntegration, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return TaskIntegration{}, err
	}
	if s.box == nil {
		return TaskIntegration{}, secretbox.ErrKeyUnavailable
	}
	in.Provider = TaskIntegrationSourceProvider(strings.ToLower(strings.TrimSpace(string(in.Provider))))
	provider, ok := s.providers[in.Provider]
	if !ok {
		return TaskIntegration{}, &ConfigurationError{Code: "integration_provider_unsupported"}
	}
	in.Provider = provider.Provider()
	if err := normalizeCommonFields(&in); err != nil {
		return TaskIntegration{}, err
	}
	if err := provider.ValidateConfig(in.Config); err != nil {
		return TaskIntegration{}, err
	}
	if strings.TrimSpace(in.Credential) == "" {
		return TaskIntegration{}, &ConfigurationError{Code: "integration_credential_required"}
	}
	if err := provider.Validate(ctx, in.Config, in.Credential); err != nil {
		return TaskIntegration{}, validationError(err)
	}
	secret, err := s.box.Seal(in.Credential)
	if err != nil {
		return TaskIntegration{}, fmt.Errorf("encrypt integration credential: %w", err)
	}
	id, err := ids.NewUUID()
	if err != nil {
		return TaskIntegration{}, err
	}
	return s.repo.Create(ctx, toTaskIntegration(id, workspaceID, in), string(secret))
}

func (s *TaskIntegrationService) Update(ctx context.Context, userID, orgID, workspaceID, sourceID string, in TaskIntegrationInput) (TaskIntegration, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return TaskIntegration{}, err
	}
	if s.box == nil {
		return TaskIntegration{}, secretbox.ErrKeyUnavailable
	}
	old, oldSecret, err := s.repo.Get(ctx, workspaceID, sourceID)
	if err != nil {
		return TaskIntegration{}, err
	}
	if in.Provider == "" {
		in.Provider = old.Provider
	}
	in.Provider = TaskIntegrationSourceProvider(strings.ToLower(strings.TrimSpace(string(in.Provider))))
	provider, ok := s.providers[in.Provider]
	if !ok {
		return TaskIntegration{}, &ConfigurationError{Code: "integration_provider_unsupported"}
	}
	in.Provider = provider.Provider()
	if len(strings.TrimSpace(string(in.Config))) == 0 || strings.TrimSpace(string(in.Config)) == "null" {
		in.Config = old.Config
	}
	if in.Name == "" {
		in.Name = old.Name
	}
	if in.WorkflowID == "" {
		in.WorkflowID = old.WorkflowID
	}
	if in.Labels == nil {
		in.Labels = old.Labels
	}
	if in.Interval == "" {
		in.Interval = old.Interval
	}
	if in.Credential == "" {
		in.Credential, err = s.box.Open([]byte(oldSecret))
		if err != nil {
			return TaskIntegration{}, err
		}
	}
	if err = normalizeCommonFields(&in); err != nil {
		return TaskIntegration{}, err
	}
	if err = provider.ValidateConfig(in.Config); err != nil {
		return TaskIntegration{}, err
	}
	if err = provider.Validate(ctx, in.Config, in.Credential); err != nil {
		return TaskIntegration{}, validationError(err)
	}
	secret, err := s.box.Seal(in.Credential)
	if err != nil {
		return TaskIntegration{}, err
	}
	return s.repo.Update(ctx, workspaceID, sourceID, toTaskIntegration(sourceID, workspaceID, in), string(secret))
}

func (s *TaskIntegrationService) Delete(ctx context.Context, userID, orgID, workspaceID, sourceID string) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	return s.repo.Delete(ctx, workspaceID, sourceID)
}

func (s *TaskIntegrationService) Validate(ctx context.Context, userID, orgID, workspaceID string, in TaskIntegrationInput) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	in.Provider = TaskIntegrationSourceProvider(strings.ToLower(strings.TrimSpace(string(in.Provider))))
	p, ok := s.providers[in.Provider]
	if !ok {
		return &ConfigurationError{Code: "integration_provider_unsupported"}
	}
	if in.Credential == "" {
		return &ConfigurationError{Code: "integration_credential_required"}
	}
	if err := normalizeCommonFields(&in); err != nil {
		return err
	}
	if err := p.ValidateConfig(in.Config); err != nil {
		return err
	}
	if err := p.Validate(ctx, in.Config, in.Credential); err != nil {
		return validationError(err)
	}
	return nil
}

func validationError(err error) error {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return err
	}
	var configurationErr *ConfigurationError
	if errors.As(err, &configurationErr) {
		return err
	}
	return &ValidationError{Code: "integration_validation_failed", Cause: err}
}

func (s *TaskIntegrationService) SyncNow(ctx context.Context, userID, orgID, workspaceID, sourceID string) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	source, encrypted, err := s.repo.Get(ctx, workspaceID, sourceID)
	if err != nil {
		return err
	}
	return s.sync(ctx, source, encrypted)
}

func (s *TaskIntegrationService) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		_ = s.syncDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *TaskIntegrationService) syncDue(ctx context.Context) error {
	sources, err := s.repo.Due(ctx, time.Now().UTC())
	if err != nil {
		return err
	}
	var errs []error
	for _, source := range sources {
		_, secret, e := s.repo.Get(ctx, source.WorkspaceID, source.ID)
		if e == nil {
			e = s.sync(ctx, source, secret)
		}
		if e != nil {
			errs = append(errs, e)
		}
	}
	return errors.Join(errs...)
}

func (s *TaskIntegrationService) sync(ctx context.Context, source TaskIntegration, encrypted string) error {
	if !source.Enabled {
		return nil
	}
	s.mu.Lock()
	if s.inFlight[source.ID] {
		s.mu.Unlock()
		return nil
	}
	s.inFlight[source.ID] = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.inFlight, source.ID); s.mu.Unlock() }()
	if s.box == nil {
		return s.recordSyncError(ctx, source.ID, secretbox.ErrKeyUnavailable)
	}
	credential, err := s.box.Open([]byte(encrypted))
	if err != nil {
		return s.recordSyncError(ctx, source.ID, err)
	}
	provider := s.providers[source.Provider]
	if provider == nil {
		return s.recordSyncError(ctx, source.ID, fmt.Errorf("provider %q is not registered", source.Provider))
	}
	items, err := provider.Discover(ctx, source.Config, credential, source.Labels)
	if err == nil {
		seen := make([]string, 0, len(items))
		for _, item := range items {
			if e := s.importer.UpsertImportedTask(ctx, source.WorkspaceID, source.ID, source.WorkflowID, string(source.Provider), item); e != nil {
				err = e
				break
			}
			seen = append(seen, item.ID)
		}
		if err == nil {
			err = s.importer.RemoveMissingImportedTasks(ctx, source.WorkspaceID, source.ID, seen)
		}
	}
	message := ""
	if err != nil {
		message = syncFailureCode(err)
	}
	if recordErr := s.repo.RecordSync(ctx, source.ID, time.Now().UTC(), message); err == nil {
		err = recordErr
	}
	return err
}

func (s *TaskIntegrationService) recordSyncError(ctx context.Context, sourceID string, cause error) error {
	_ = s.repo.RecordSync(ctx, sourceID, time.Now().UTC(), syncFailureCode(cause))
	return cause
}

func syncFailureCode(err error) string {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) && validationErr.Code != "" {
		return validationErr.Code
	}
	var configurationErr *ConfigurationError
	if errors.As(err, &configurationErr) && configurationErr.Code != "" {
		return configurationErr.Code
	}
	return "integration_sync_failed"
}

func (s *TaskIntegrationService) authorize(ctx context.Context, userID, orgID, workspaceID string) error {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role == "admin" {
		return nil
	}
	role, err := s.organizations.RoleForUser(ctx, userID, orgID)
	if err != nil {
		return err
	}
	if role == "owner" || role == "admin" {
		return nil
	}
	return modelerrors.ErrForbidden
}

func normalizeCommonFields(in *TaskIntegrationInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.WorkflowID = strings.TrimSpace(in.WorkflowID)
	in.Labels = normalizeValues(in.Labels)
	if in.Name == "" || len(in.Name) > 200 {
		return &ConfigurationError{Code: "integration_name_required"}
	}
	if in.WorkflowID == "" {
		return &ConfigurationError{Code: "integration_workflow_required"}
	}
	if in.Interval == "" {
		in.Interval = DefaultInterval.String()
	}
	d, err := time.ParseDuration(in.Interval)
	if err != nil || d < time.Minute || d > 24*time.Hour {
		return &ConfigurationError{Code: "integration_interval_invalid"}
	}
	in.Interval = d.String()
	return nil
}

func toTaskIntegration(id, workspaceID string, in TaskIntegrationInput) TaskIntegration {
	return TaskIntegration{ID: id, WorkspaceID: workspaceID, WorkflowID: in.WorkflowID, Provider: in.Provider, Name: in.Name, Enabled: in.Enabled, Config: in.Config, Labels: in.Labels, Interval: in.Interval}
}

func normalizeValues(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
