package organization

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

type OrganizationService struct{ repository Repository }

func NewOrganizationService(repository Repository) *OrganizationService {
	return &OrganizationService{repository: repository}
}

func normalizeName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	return name, name != "" && len(name) <= 200
}

func (s *OrganizationService) RoleForUser(ctx context.Context, userID, organizationID string) (string, error) {
	return s.repository.OrganizationRole(ctx, userID, organizationID)
}

func (s *OrganizationService) ListOrganizations(ctx context.Context, userID string) ([]Organization, error) {
	return s.repository.Organizations(ctx, userID)
}

func (s *OrganizationService) GetOrganization(ctx context.Context, userID, orgID string) (Organization, error) {
	return s.repository.Organization(ctx, userID, orgID)
}

func (s *OrganizationService) CreateOrganization(ctx context.Context, userID, name string) (Organization, error) {
	var valid bool
	if name, valid = normalizeName(name); !valid {
		return Organization{}, modelerrors.ErrInvalid
	}
	id, err := ids.NewUUID()
	if err != nil {
		return Organization{}, fmt.Errorf("generate organization ID: %w", err)
	}
	return s.repository.CreateOrganization(ctx, id, userID, name)
}

func (s *OrganizationService) UpdateOrganization(ctx context.Context, userID, orgID, name string) (Organization, error) {
	var valid bool
	if name, valid = normalizeName(name); !valid {
		return Organization{}, modelerrors.ErrInvalid
	}
	role, err := s.repository.OrganizationRole(ctx, userID, orgID)
	if err != nil {
		return Organization{}, err
	}
	if role != "owner" && role != "admin" {
		return Organization{}, modelerrors.ErrForbidden
	}
	return s.repository.UpdateOrganization(ctx, orgID, userID, name)
}

func (s *OrganizationService) DeleteOrganization(ctx context.Context, userID, orgID string) error {
	role, err := s.repository.OrganizationRole(ctx, userID, orgID)
	if err != nil {
		return err
	}
	if role != "owner" {
		return modelerrors.ErrForbidden
	}
	return s.repository.DeleteOrganization(ctx, orgID)
}

func (s *OrganizationService) ListOrganizationMembers(ctx context.Context, userID, orgID string) ([]Membership, error) {
	if _, err := s.repository.Organization(ctx, userID, orgID); err != nil {
		return nil, err
	}
	return s.repository.OrganizationMembers(ctx, orgID)
}

func (s *OrganizationService) SetOrganizationMember(ctx context.Context, actorID, orgID, targetID, role string) error {
	if role != "owner" && role != "admin" && role != "member" {
		return modelerrors.ErrInvalid
	}
	actorRole, err := s.repository.OrganizationRole(ctx, actorID, orgID)
	if err != nil {
		return err
	}
	if actorRole != "owner" && actorRole != "admin" {
		return modelerrors.ErrForbidden
	}
	if actorRole == "admin" {
		if targetRole, e := s.repository.OrganizationRole(ctx, targetID, orgID); e == nil && targetRole == "owner" {
			return modelerrors.ErrForbidden
		} else if e != nil && !errors.Is(e, modelerrors.ErrNotFound) {
			return e
		}
	}
	if role == "owner" && actorRole != "owner" {
		return modelerrors.ErrForbidden
	}
	return s.repository.SetOrganizationMember(ctx, actorID, orgID, targetID, role)
}

func (s *OrganizationService) RemoveOrganizationMember(ctx context.Context, actorID, orgID, targetID string) error {
	actorRole, err := s.repository.OrganizationRole(ctx, actorID, orgID)
	if err != nil {
		return err
	}
	if actorRole != "owner" && actorRole != "admin" {
		return modelerrors.ErrForbidden
	}
	if actorRole == "admin" {
		targetRole, e := s.repository.OrganizationRole(ctx, targetID, orgID)
		if e != nil {
			return e
		}
		if targetRole == "owner" {
			return modelerrors.ErrForbidden
		}
	}
	return s.repository.RemoveOrganizationMember(ctx, actorID, orgID, targetID)
}
