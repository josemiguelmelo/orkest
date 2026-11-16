package identity

import (
	"context"
	"testing"
	"time"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	identitymodel "github.com/josemiguelmelo/orkest/internal/server/identity/model"
	"github.com/josemiguelmelo/orkest/internal/server/organization"
)

func TestLocalPasswordHash(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) == 0 || hash[:8] != "argon2id" {
		t.Fatalf("password hash does not identify Argon2id: %q", hash)
	}
	if !verifyPassword(hash, "correct horse battery staple") {
		t.Fatal("password hash did not verify")
	}
	if verifyPassword(hash, "incorrect password") {
		t.Fatal("incorrect password verified")
	}
}

type localAccountRepositoryStub struct {
	linkedSubject string
	linkedEmail   string
	linkCalled    bool
}

func (r *localAccountRepositoryStub) ProvisionLocalAccount(context.Context, string, string, string, string, string, string, string, string) (identitymodel.User, error) {
	return identitymodel.User{}, nil
}

func (r *localAccountRepositoryStub) BootstrapLocalAdminRequired(context.Context, string) (bool, error) {
	return false, nil
}

func (r *localAccountRepositoryStub) BootstrapLocalAdmin(context.Context, string, string, string, string, string, string) (identitymodel.User, bool, error) {
	return identitymodel.User{}, false, nil
}

func (r *localAccountRepositoryStub) LocalCredentialByUsername(context.Context, string) (LocalCredential, error) {
	return LocalCredential{}, nil
}

func (r *localAccountRepositoryStub) LocalCredentialByUserID(context.Context, string) (LocalCredential, error) {
	return LocalCredential{}, ErrNoLocalPassword
}

func (r *localAccountRepositoryStub) UpdateLocalPassword(context.Context, string, string, string) error {
	return nil
}

func (r *localAccountRepositoryStub) HasLocalPassword(context.Context, string) (bool, error) {
	return false, nil
}

func (r *localAccountRepositoryStub) CreateLocalSession(context.Context, []byte, string, time.Time) error {
	return nil
}

func (r *localAccountRepositoryStub) UserForLocalSession(context.Context, []byte) (identitymodel.User, error) {
	return identitymodel.User{}, nil
}

func (r *localAccountRepositoryStub) RevokeLocalSession(context.Context, []byte) error { return nil }

func (r *localAccountRepositoryStub) LinkKeycloakIdentityByEmail(_ context.Context, subject, email string) (identitymodel.User, error) {
	r.linkCalled, r.linkedSubject, r.linkedEmail = true, subject, email
	return identitymodel.User{ID: "user"}, nil
}

func (r *localAccountRepositoryStub) HasLocalAccountEmail(context.Context, string) (bool, error) {
	return false, nil
}

func TestKeycloakLinkRequiresVerifiedEmail(t *testing.T) {
	repository := &localAccountRepositoryStub{}
	service := NewLocalAuthenticationService(repository, nil, nil)
	if _, err := service.LinkKeycloakIdentity(context.Background(), "subject", "person@example.test", false); err != ErrInvalidCredentials {
		t.Fatalf("unverified email link error = %v", err)
	}
	if repository.linkCalled {
		t.Fatal("repository linked an unverified email")
	}
	if _, err := service.LinkKeycloakIdentity(context.Background(), "subject", "person@example.test", true); err != nil {
		t.Fatal(err)
	}
	if repository.linkedSubject != "subject" || repository.linkedEmail != "person@example.test" {
		t.Fatalf("linked %q / %q", repository.linkedSubject, repository.linkedEmail)
	}
}

type registrationRepositoryStub struct {
	UserRegistrationRepository
	platformAdmin bool
}

func (r registrationRepositoryStub) IsPlatformAdmin(context.Context, string) (bool, error) {
	return r.platformAdmin, nil
}

type organizationRoleRepositoryStub struct {
	organization.OrganizationRepository
	role string
}

func (r organizationRoleRepositoryStub) OrganizationRole(context.Context, string, string) (string, error) {
	return r.role, nil
}

func TestProvisionLocalAccountRequiresAdministrator(t *testing.T) {
	users := NewUserRegistrationService(registrationRepositoryStub{})
	organizations := organization.NewOrganizationService(organizationRoleRepositoryStub{role: "member"})
	service := NewLocalAuthenticationService(nil, users, organizations)
	_, err := service.Provision(context.Background(), "actor", "org", "new-user", "new@example.test", "long-enough-password", "member")
	if err != modelerrors.ErrForbidden {
		t.Fatalf("provision error = %v, want forbidden", err)
	}
}
