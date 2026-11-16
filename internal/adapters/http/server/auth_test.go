package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/josemiguelmelo/orkest/internal/server/identity"
	identitymodel "github.com/josemiguelmelo/orkest/internal/server/identity/model"
	"github.com/josemiguelmelo/orkest/internal/server/organization"
	organizationmodel "github.com/josemiguelmelo/orkest/internal/server/organization/model"
)

type adminProfileRepository struct {
	identity.UserRegistrationRepository
	admin bool
	err   error
}

func (r adminProfileRepository) IsPlatformAdmin(context.Context, string) (bool, error) {
	return r.admin, r.err
}

type profileOrganizationsRepository struct {
	organization.OrganizationRepository
}

func (profileOrganizationsRepository) Organizations(context.Context, string) ([]organizationmodel.Organization, error) {
	return []organizationmodel.Organization{}, nil
}

func TestCurrentUserPlatformAdmin(t *testing.T) {
	for _, admin := range []bool{false, true} {
		auth := platformAuth{users: identity.NewUserRegistrationService(adminProfileRepository{admin: admin}), organizations: organization.NewOrganizationService(profileOrganizationsRepository{})}
		request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		request = request.WithContext(context.WithValue(request.Context(), userContextKey{}, identitymodel.User{ID: "user", Username: "name"}))
		recorder := httptest.NewRecorder()
		auth.currentUser(recorder, request)
		var result struct {
			PlatformAdmin bool               `json:"platformAdmin"`
			User          identitymodel.User `json:"user"`
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
	auth := platformAuth{users: identity.NewUserRegistrationService(adminProfileRepository{err: errors.New("private database detail")}), organizations: organization.NewOrganizationService(profileOrganizationsRepository{})}
	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request = request.WithContext(context.WithValue(request.Context(), userContextKey{}, identitymodel.User{ID: "user"}))
	recorder := httptest.NewRecorder()
	auth.currentUser(recorder, request)
	if recorder.Code != http.StatusInternalServerError || recorder.Body.String() != "{\"error\":\"internal_error\"}\n" {
		t.Fatalf("unexpected response %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestAuthenticationMethodsAlwaysIncludesBothMethods(t *testing.T) {
	auth := platformAuth{}
	recorder := httptest.NewRecorder()
	auth.methods(recorder, httptest.NewRequest(http.MethodGet, "/auth/methods", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "{\"keycloak\":true,\"local\":true}\n" {
		t.Fatalf("methods response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestLocalLoginRateLimitAndSuccessfulReset(t *testing.T) {
	auth := &platformAuth{}
	now := time.Now()
	for i := 0; i < 10; i++ {
		if !auth.allowLocalLogin("127.0.0.1", now) {
			t.Fatalf("attempt %d was unexpectedly limited", i+1)
		}
	}
	if auth.allowLocalLogin("127.0.0.1", now) {
		t.Fatal("11th attempt was not limited")
	}
	auth.clearLocalLoginFailures("127.0.0.1")
	if !auth.allowLocalLogin("127.0.0.1", now) {
		t.Fatal("successful login reset did not clear the limit")
	}
}

func TestRefreshSessionRequestsOpenIDTokenAndRetainsRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("scope"); got != "openid profile email" {
			t.Errorf("scope = %q", got)
		}
		if got := r.Form.Get("refresh_token"); got != "old-refresh" {
			t.Errorf("refresh_token = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access","id_token":"new-id","expires_in":300}`))
	}))
	defer server.Close()

	auth := platformAuth{
		config: oauth2.Config{
			ClientID: "client", ClientSecret: "secret",
			Endpoint: oauth2.Endpoint{TokenURL: server.URL},
			Scopes:   []string{"openid", "profile", "email"},
		},
		httpClient: server.Client(),
	}
	token, err := auth.refreshSession(context.Background(), "old-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := token.Extra("id_token").(string); got != "new-id" {
		t.Fatalf("id_token = %q", got)
	}
	if token.RefreshToken != "old-refresh" {
		t.Fatalf("refresh token = %q", token.RefreshToken)
	}
}

func TestShouldRefreshSessionWhenSessionCookieIsAbsent(t *testing.T) {
	if !shouldRefreshSession("", "", nil) {
		t.Fatal("expected refresh when the browser omits the expired session cookie")
	}
	if shouldRefreshSession("", "Bearer supplied-token", nil) {
		t.Fatal("must not refresh a bearer-authenticated request")
	}
}
