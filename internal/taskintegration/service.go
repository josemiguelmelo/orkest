package taskintegration

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type Service struct {
	repo          Repository
	box           *SecretBox
	providers     map[string]SourceIntegration
	workspaces    WorkspaceAccess
	organizations OrganizationAccess
	importer      TaskImporter
	mu            sync.Mutex
	inFlight      map[string]bool
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error)
}
type OrganizationAccess interface {
	RoleForUser(context.Context, string, string) (string, error)
}

func NewService(repo Repository, box *SecretBox, workspaces WorkspaceAccess, organizations OrganizationAccess, importer TaskImporter, providers ...SourceIntegration) *Service {
	s := &Service{repo: repo, box: box, workspaces: workspaces, organizations: organizations, importer: importer, providers: map[string]SourceIntegration{}, inFlight: map[string]bool{}}
	for _, p := range providers {
		s.providers[p.Name()] = p
	}
	return s
}

func (s *Service) List(ctx context.Context, userID, orgID, workspaceID string) ([]Source, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, workspaceID)
}

func (s *Service) Create(ctx context.Context, userID, orgID, workspaceID string, in SourceInput) (Source, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Source{}, err
	}
	if s.box == nil {
		return Source{}, ErrEncryptionKeyUnavailable
	}
	provider, ok := s.providers[strings.ToLower(strings.TrimSpace(in.Provider))]
	if !ok {
		return Source{}, &ConfigurationError{Code: "integration_provider_unsupported"}
	}
	in.Provider = provider.Name()
	if err := normalize(&in); err != nil {
		return Source{}, err
	}
	if strings.TrimSpace(in.Credential) == "" {
		return Source{}, &ConfigurationError{Code: "integration_credential_required"}
	}
	if err := provider.Validate(ctx, in.Config, in.Credential); err != nil {
		return Source{}, validationError(err)
	}
	secret, err := s.box.Seal(in.Credential)
	if err != nil {
		return Source{}, fmt.Errorf("encrypt integration credential: %w", err)
	}
	id, err := ids.NewUUID()
	if err != nil {
		return Source{}, err
	}
	return s.repo.Create(ctx, toSource(id, workspaceID, in), string(secret))
}

func (s *Service) Update(ctx context.Context, userID, orgID, workspaceID, sourceID string, in SourceInput) (Source, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Source{}, err
	}
	if s.box == nil {
		return Source{}, ErrEncryptionKeyUnavailable
	}
	old, oldSecret, err := s.repo.Get(ctx, workspaceID, sourceID)
	if err != nil {
		return Source{}, err
	}
	if in.Provider == "" {
		in.Provider = old.Provider
	}
	provider, ok := s.providers[strings.ToLower(strings.TrimSpace(in.Provider))]
	if !ok {
		return Source{}, &ConfigurationError{Code: "integration_provider_unsupported"}
	}
	in.Provider = provider.Name()
	if in.Config.BaseURL == "" && in.Config.ProjectURL == "" && len(in.Config.Projects) == 0 {
		in.Config = old.Config
	}
	if in.Name == "" {
		in.Name = old.Name
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
			return Source{}, err
		}
	}
	if err = normalize(&in); err != nil {
		return Source{}, err
	}
	if err = provider.Validate(ctx, in.Config, in.Credential); err != nil {
		return Source{}, validationError(err)
	}
	secret, err := s.box.Seal(in.Credential)
	if err != nil {
		return Source{}, err
	}
	return s.repo.Update(ctx, workspaceID, sourceID, toSource(sourceID, workspaceID, in), string(secret))
}

func (s *Service) Delete(ctx context.Context, userID, orgID, workspaceID, sourceID string) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	return s.repo.Delete(ctx, workspaceID, sourceID)
}

func (s *Service) Validate(ctx context.Context, userID, orgID, workspaceID string, in SourceInput) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	p, ok := s.providers[strings.ToLower(strings.TrimSpace(in.Provider))]
	if !ok {
		return &ConfigurationError{Code: "integration_provider_unsupported"}
	}
	if in.Credential == "" {
		return &ConfigurationError{Code: "integration_credential_required"}
	}
	if err := normalize(&in); err != nil {
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
	return &ValidationError{Code: "integration_validation_failed", Cause: err}
}

func (s *Service) SyncNow(ctx context.Context, userID, orgID, workspaceID, sourceID string) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	source, encrypted, err := s.repo.Get(ctx, workspaceID, sourceID)
	if err != nil {
		return err
	}
	return s.sync(ctx, source, encrypted)
}

func (s *Service) Run(ctx context.Context) {
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

func (s *Service) syncDue(ctx context.Context) error {
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

func (s *Service) sync(ctx context.Context, source Source, encrypted string) error {
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
		return s.recordSyncError(ctx, source.ID, ErrEncryptionKeyUnavailable)
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
			if e := s.importer.UpsertImportedTask(ctx, source.WorkspaceID, source.ID, source.Provider, item); e != nil {
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
		message = "synchronization failed"
	}
	if recordErr := s.repo.RecordSync(ctx, source.ID, time.Now().UTC(), message); err == nil {
		err = recordErr
	}
	return err
}

func (s *Service) recordSyncError(ctx context.Context, sourceID string, cause error) error {
	_ = s.repo.RecordSync(ctx, sourceID, time.Now().UTC(), "synchronization failed")
	return cause
}

func (s *Service) authorize(ctx context.Context, userID, orgID, workspaceID string) error {
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

func normalize(in *SourceInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Labels = normalizeValues(in.Labels)
	if in.Name == "" || len(in.Name) > 200 {
		return &ConfigurationError{Code: "integration_name_required"}
	}
	if in.Interval == "" {
		in.Interval = DefaultInterval.String()
	}
	d, err := time.ParseDuration(in.Interval)
	if err != nil || d < time.Minute || d > 24*time.Hour {
		return &ConfigurationError{Code: "integration_interval_invalid"}
	}
	in.Interval = d.String()
	in.Config.BaseURL = strings.TrimRight(strings.TrimSpace(in.Config.BaseURL), "/")
	in.Config.ProjectURL = strings.TrimRight(strings.TrimSpace(in.Config.ProjectURL), "/")
	if in.Config.BaseURL != "" {
		u, e := url.ParseRequestURI(in.Config.BaseURL)
		if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return &ConfigurationError{Code: "jira_base_url_invalid"}
		}
	}
	in.Config.Projects = normalizeValues(in.Config.Projects)
	switch strings.ToLower(in.Provider) {
	case ProviderGitHub:
		if in.Config.ProjectURL == "" {
			return &ConfigurationError{Code: "github_project_url_required"}
		}
		if _, err := ParseGitHubProjectURL(in.Config.ProjectURL); err != nil {
			return &ConfigurationError{Code: "github_project_url_invalid"}
		}
		if in.Config.BaseURL != "" || len(in.Config.Projects) != 0 {
			return &ConfigurationError{Code: "github_config_invalid"}
		}
	case ProviderJira:
		if in.Config.BaseURL == "" {
			return &ConfigurationError{Code: "jira_base_url_required"}
		}
		if len(in.Config.Projects) == 0 {
			return &ConfigurationError{Code: "jira_projects_required"}
		}
		if in.Config.ProjectURL != "" {
			return &ConfigurationError{Code: "jira_config_invalid"}
		}
	default:
		return &ConfigurationError{Code: "integration_provider_unsupported"}
	}
	return nil
}

func toSource(id, workspaceID string, in SourceInput) Source {
	return Source{ID: id, WorkspaceID: workspaceID, Provider: in.Provider, Name: in.Name, Enabled: in.Enabled, Config: in.Config, Labels: in.Labels, Interval: in.Interval}
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
