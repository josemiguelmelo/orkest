package organization

import (
	"context"

	organizationmodel "github.com/josemiguelmelo/orkest/internal/server/organization/model"
)

type OrganizationRepository interface {
	Organizations(context.Context, string) ([]organizationmodel.Organization, error)
	Organization(context.Context, string, string) (organizationmodel.Organization, error)
	CreateOrganization(context.Context, string, string, string) (organizationmodel.Organization, error)
	UpdateOrganization(context.Context, string, string, string) (organizationmodel.Organization, error)
	DeleteOrganization(context.Context, string) error
	OrganizationRole(context.Context, string, string) (string, error)
	OrganizationMembers(context.Context, string) ([]organizationmodel.Membership, error)
	SetOrganizationMember(context.Context, string, string, string, string) error
	RemoveOrganizationMember(context.Context, string, string, string) error
}
