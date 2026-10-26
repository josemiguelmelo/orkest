package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/identity"
	"github.com/josemiguelmelo/orkest/internal/organization"
)

type adminProfileRepository struct {
	identity.Repository
	admin bool
	err   error
}

func (r adminProfileRepository) IsPlatformAdmin(context.Context, string) (bool, error) {
	return r.admin, r.err
}

type profileOrganizationsRepository struct{ organization.Repository }

func (profileOrganizationsRepository) Organizations(context.Context, string) ([]organization.Organization, error) {
	return []organization.Organization{}, nil
}

func TestCurrentUserPlatformAdmin(t *testing.T) {
	for _, admin := range []bool{false, true} {
		auth := oidcAuth{users: identity.NewUserRegistrationService(adminProfileRepository{admin: admin}), organizations: organization.NewOrganizationService(profileOrganizationsRepository{})}
		request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		request = request.WithContext(context.WithValue(request.Context(), userContextKey{}, identity.User{ID: "user", Username: "name"}))
		recorder := httptest.NewRecorder()
		auth.currentUser(recorder, request)
		var result struct {
			PlatformAdmin bool          `json:"platformAdmin"`
			User          identity.User `json:"user"`
		}
		if recorder.Code != http.StatusOK {
			t.Fatalf("status %d", recorder.Code)
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.PlatformAdmin != admin || result.User.ID != "user" {
			t.Fatalf("unexpected profile: %+v", result)
		}
	}
}

func TestCurrentUserAdminLookupFailure(t *testing.T) {
	auth := oidcAuth{users: identity.NewUserRegistrationService(adminProfileRepository{err: errors.New("private database detail")}), organizations: organization.NewOrganizationService(profileOrganizationsRepository{})}
	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request = request.WithContext(context.WithValue(request.Context(), userContextKey{}, identity.User{ID: "user"}))
	recorder := httptest.NewRecorder()
	auth.currentUser(recorder, request)
	if recorder.Code != http.StatusInternalServerError || recorder.Body.String() != "{\"error\":\"internal_error\"}\n" {
		t.Fatalf("unexpected response %d %q", recorder.Code, recorder.Body.String())
	}
}
