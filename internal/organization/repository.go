package organization

import "context"

type Repository interface {
	Organizations(context.Context, string) ([]Organization, error)
	Organization(context.Context, string, string) (Organization, error)
	CreateOrganization(context.Context, string, string, string) (Organization, error)
	UpdateOrganization(context.Context, string, string, string) (Organization, error)
	DeleteOrganization(context.Context, string) error
	OrganizationRole(context.Context, string, string) (string, error)
	OrganizationMembers(context.Context, string) ([]Membership, error)
	SetOrganizationMember(context.Context, string, string, string, string) error
	RemoveOrganizationMember(context.Context, string, string, string) error
}
