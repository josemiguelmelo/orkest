package organization

import (
	"context"
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	organizationmodel "github.com/josemiguelmelo/orkest/internal/server/organization/model"
)

type organizationRepositoryStub struct {
	roles          map[string]string
	setMemberCalls int
}

func (r *organizationRepositoryStub) Organizations(context.Context, string) ([]organizationmodel.Organization, error) {
	return nil, nil
}

func (r *organizationRepositoryStub) Organization(_ context.Context, userID, organizationID string) (organizationmodel.Organization, error) {
	role, err := r.OrganizationRole(context.Background(), userID, organizationID)
	return organizationmodel.Organization{ID: organizationID, Role: role}, err
}

func (r *organizationRepositoryStub) CreateOrganization(context.Context, string, string, string) (organizationmodel.Organization, error) {
	return organizationmodel.Organization{}, nil
}

func (r *organizationRepositoryStub) UpdateOrganization(context.Context, string, string, string) (organizationmodel.Organization, error) {
	return organizationmodel.Organization{}, nil
}

func (r *organizationRepositoryStub) DeleteOrganization(context.Context, string) error { return nil }

func (r *organizationRepositoryStub) OrganizationRole(_ context.Context, userID, organizationID string) (string, error) {
	role, ok := r.roles[organizationID+":"+userID]
	if !ok {
		return "", modelerrors.ErrNotFound
	}
	return role, nil
}

func (r *organizationRepositoryStub) OrganizationMembers(context.Context, string) ([]organizationmodel.Membership, error) {
	return nil, nil
}

func (r *organizationRepositoryStub) SetOrganizationMember(context.Context, string, string, string, string) error {
	r.setMemberCalls++
	return nil
}

func (r *organizationRepositoryStub) RemoveOrganizationMember(context.Context, string, string, string) error {
	return nil
}

func TestAdminCannotChangeOwnerMembership(t *testing.T) {
	repository := &organizationRepositoryStub{roles: map[string]string{"org:admin": "admin", "org:owner": "owner"}}
	err := NewOrganizationService(repository).SetOrganizationMember(context.Background(), "admin", "org", "owner", "member")
	if !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if repository.setMemberCalls != 0 {
		t.Fatal("forbidden update reached repository")
	}
}
