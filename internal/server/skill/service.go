package skill

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	skillmodel "github.com/josemiguelmelo/orkest/internal/server/skill/model"
	"go.yaml.in/yaml/v4"
)

var skillIDPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type CatalogService struct {
	repository     CatalogRepository
	organizations  OrganizationAccess
	workspaces     WorkspaceAccess
	platformAdmins PlatformAdminAccess
}

func NewCatalogService(repository CatalogRepository, organizations OrganizationAccess, workspaces WorkspaceAccess, platformAdmins PlatformAdminAccess) *CatalogService {
	return &CatalogService{repository: repository, organizations: organizations, workspaces: workspaces, platformAdmins: platformAdmins}
}

func (s *CatalogService) List(ctx context.Context, userID string, scope skillmodel.Scope, orgID, workspaceID string) ([]skillmodel.Skill, error) {
	if err := s.authorizeRead(ctx, userID, scope, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.Skills(ctx, orgID, workspaceID)
}

func (s *CatalogService) Get(ctx context.Context, userID, orgID, workspaceID, id string) (skillmodel.Skill, error) {
	if workspaceID != "" {
		if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
			return skillmodel.Skill{}, err
		}
	} else if orgID != "" {
		if _, err := s.organizations.RoleForUser(ctx, userID, orgID); err != nil {
			return skillmodel.Skill{}, err
		}
	}
	return s.repository.Skill(ctx, orgID, workspaceID, id)
}

func (s *CatalogService) Create(ctx context.Context, userID string, scope skillmodel.Scope, orgID, workspaceID, skillMD string) (skillmodel.Skill, error) {
	id, err := validateContent(skillMD)
	if err != nil {
		return skillmodel.Skill{}, err
	}
	if err = s.authorizeWrite(ctx, userID, scope, orgID, workspaceID); err != nil {
		return skillmodel.Skill{}, err
	}
	return s.repository.CreateSkill(ctx, id, scope, orgID, workspaceID, skillMD)
}

func (s *CatalogService) Update(ctx context.Context, userID, orgID, workspaceID, id, skillMD string) (skillmodel.Skill, error) {
	contentID, err := validateContent(skillMD)
	if err != nil {
		return skillmodel.Skill{}, err
	}
	if contentID != id {
		return skillmodel.Skill{}, modelerrors.ErrInvalid
	}
	current, err := s.Get(ctx, userID, orgID, workspaceID, id)
	if err != nil {
		return skillmodel.Skill{}, err
	}
	if err = s.authorizeWrite(ctx, userID, current.Scope, current.OrganizationID, current.WorkspaceID); err != nil {
		return skillmodel.Skill{}, err
	}
	return s.repository.UpdateSkill(ctx, id, skillMD)
}

func (s *CatalogService) Delete(ctx context.Context, userID, orgID, workspaceID, id string) error {
	current, err := s.Get(ctx, userID, orgID, workspaceID, id)
	if err != nil {
		return err
	}
	if err = s.authorizeWrite(ctx, userID, current.Scope, current.OrganizationID, current.WorkspaceID); err != nil {
		return err
	}
	return s.repository.DeleteSkill(ctx, id)
}

func (s *CatalogService) ValidateAgentSkills(ctx context.Context, orgID, workspaceID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	items, err := s.repository.VisibleSkills(ctx, orgID, workspaceID, ids)
	if err != nil {
		return err
	}
	if len(items) != len(ids) {
		return modelerrors.ErrInvalid
	}
	return nil
}

func (s *CatalogService) ResolveForAgent(ctx context.Context, orgID, workspaceID string, ids []string) ([]skillmodel.Skill, error) {
	return s.repository.VisibleSkills(ctx, orgID, workspaceID, ids)
}

func (s *CatalogService) authorizeRead(ctx context.Context, userID string, scope skillmodel.Scope, orgID, workspaceID string) error {
	switch scope {
	case skillmodel.ScopeGlobal:
		return nil
	case skillmodel.ScopeOrganization:
		_, err := s.organizations.RoleForUser(ctx, userID, orgID)
		return err
	case skillmodel.ScopeWorkspace:
		_, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
		return err
	default:
		return modelerrors.ErrInvalid
	}
}

func (s *CatalogService) authorizeWrite(ctx context.Context, userID string, scope skillmodel.Scope, orgID, workspaceID string) error {
	switch scope {
	case skillmodel.ScopeGlobal:
		allowed, err := s.platformAdmins.IsPlatformAdmin(ctx, userID)
		if err != nil {
			return err
		}
		if !allowed {
			return modelerrors.ErrForbidden
		}
	case skillmodel.ScopeOrganization:
		role, err := s.organizations.RoleForUser(ctx, userID, orgID)
		if err != nil {
			return err
		}
		if role != "owner" && role != "admin" {
			return modelerrors.ErrForbidden
		}
	case skillmodel.ScopeWorkspace:
		w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
		if err != nil {
			return err
		}
		if w.Role != "admin" && w.Role != "owner" {
			return modelerrors.ErrForbidden
		}
	default:
		return modelerrors.ErrInvalid
	}
	return nil
}

func validateContent(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" || len(content) > 100000 {
		return "", modelerrors.ErrInvalid
	}
	if !strings.HasPrefix(content, "---\n") {
		return "", modelerrors.ErrInvalid
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return "", modelerrors.ErrInvalid
	}
	closingEnd := 4 + end + 4
	if closingEnd < len(content) && content[closingEnd] != '\n' {
		return "", modelerrors.ErrInvalid
	}
	frontmatter := content[4 : 4+end]
	var metadata struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil {
		return "", modelerrors.ErrInvalid
	}
	metadata.Name = strings.TrimSpace(metadata.Name)
	metadata.Description = strings.TrimSpace(metadata.Description)
	if len(metadata.Name) == 0 || len(metadata.Name) > 64 || !skillIDPattern.MatchString(metadata.Name) || len(metadata.Description) == 0 || len(metadata.Description) > 1024 || strings.TrimSpace(content[closingEnd:]) == "" {
		return "", modelerrors.ErrInvalid
	}
	return metadata.Name, nil
}

func ValidateID(id string) error {
	if len(id) == 0 || len(id) > 64 || !skillIDPattern.MatchString(id) {
		return fmt.Errorf("%w: invalid skill ID", modelerrors.ErrInvalid)
	}
	return nil
}
