package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/josemiguelmelo/orkest/internal/platform/httplog"
	"github.com/josemiguelmelo/orkest/internal/server/identity"
	identitymodel "github.com/josemiguelmelo/orkest/internal/server/identity/model"
	"github.com/josemiguelmelo/orkest/internal/server/organization"
	organizationmodel "github.com/josemiguelmelo/orkest/internal/server/organization/model"
	"golang.org/x/oauth2"
)

const (
	sessionCookie = "orkest_session"
	refreshCookie = "orkest_refresh"
)

type platformAuth struct {
	config        oauth2.Config
	httpClient    *http.Client
	verifier      *oidc.IDTokenVerifier
	secure        bool
	users         *identity.UserRegistrationService
	organizations *organization.OrganizationService
	local         *identity.LocalAuthenticationService
	loginMu       sync.Mutex
	loginFailures map[string][]time.Time
}

type userContextKey struct{}

func newPlatformAuth(ctx context.Context, users *identity.UserRegistrationService, organizations *organization.OrganizationService, local *identity.LocalAuthenticationService) (*platformAuth, error) {
	if local == nil {
		return nil, fmt.Errorf("local authentication service is required")
	}
	issuer := envOr("OIDC_ISSUER_URL", "http://localhost:8082/realms/orkest")
	clientID := envOr("OIDC_CLIENT_ID", "orkest-web")
	httpClient, err := oidcHTTPClient(issuer, os.Getenv("OIDC_INTERNAL_ISSUER_URL"))
	if err != nil {
		return nil, fmt.Errorf("configure OIDC HTTP client: %w", err)
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	redirectURL := envOr("OIDC_REDIRECT_URL", "http://localhost:8080/auth/callback")
	endpoint := provider.Endpoint()
	endpoint.AuthURL, err = publicOIDCEndpoint(issuer, endpoint.AuthURL)
	if err != nil {
		return nil, fmt.Errorf("configure public OIDC authorization endpoint: %w", err)
	}
	return &platformAuth{
		config: oauth2.Config{
			ClientID: clientID, ClientSecret: envOr("OIDC_CLIENT_SECRET", "orkest-local-secret"),
			Endpoint: endpoint, RedirectURL: redirectURL,
			Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
		},
		httpClient: httpClient,
		verifier:   provider.Verifier(&oidc.Config{ClientID: clientID}), users: users, organizations: organizations,
		local: local, secure: strings.EqualFold(envOr("ORKEST_COOKIE_SECURE", os.Getenv("OIDC_COOKIE_SECURE")), "true"),
	}, nil
}

func publicOIDCEndpoint(issuer, endpoint string) (string, error) {
	publicURL, err := url.Parse(issuer)
	if err != nil {
		return "", fmt.Errorf("parse public issuer: %w", err)
	}
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse authorization endpoint: %w", err)
	}
	if publicURL.Scheme == "" || publicURL.Host == "" || endpointURL.Scheme == "" || endpointURL.Host == "" {
		return "", fmt.Errorf("issuer and authorization endpoint must be absolute URLs")
	}
	endpointURL.Scheme = publicURL.Scheme
	endpointURL.Host = publicURL.Host
	return endpointURL.String(), nil
}

func oidcHTTPClient(issuer, internalIssuer string) (*http.Client, error) {
	transport := http.DefaultTransport
	if internalIssuer != "" {
		publicURL, err := url.Parse(issuer)
		if err != nil {
			return nil, fmt.Errorf("parse public issuer: %w", err)
		}
		internalURL, err := url.Parse(internalIssuer)
		if err != nil {
			return nil, fmt.Errorf("parse internal issuer: %w", err)
		}
		if publicURL.Scheme == "" || publicURL.Host == "" || internalURL.Scheme == "" || internalURL.Host == "" {
			return nil, fmt.Errorf("public and internal issuers must be absolute URLs")
		}
		transport = &oidcIssuerTransport{base: http.DefaultTransport, public: publicURL, internal: internalURL}
	}
	return &http.Client{Transport: transport}, nil
}

type oidcIssuerTransport struct {
	base     http.RoundTripper
	public   *url.URL
	internal *url.URL
}

func (t *oidcIssuerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme != t.public.Scheme || request.URL.Host != t.public.Host {
		return t.base.RoundTrip(request)
	}
	clone := request.Clone(request.Context())
	urlCopy := *request.URL
	urlCopy.Scheme = t.internal.Scheme
	urlCopy.Host = t.internal.Host
	clone.URL = &urlCopy
	return t.base.RoundTrip(clone)
}

func (a *platformAuth) login(w http.ResponseWriter, r *http.Request) {
	state, err := randomState()
	if err != nil {
		http.Error(w, "could not start login", http.StatusInternalServerError)
		return
	}
	nonce, err := randomState()
	if err != nil {
		http.Error(w, "could not start login", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "orkest_oauth_state", Value: state, Path: "/auth", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: 300})
	http.SetCookie(w, &http.Cookie{Name: "orkest_oauth_nonce", Value: nonce, Path: "/auth", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: 300})
	http.Redirect(w, r, a.config.AuthCodeURL(state, oidc.Nonce(nonce)), http.StatusFound)
}

func (a *platformAuth) methods(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"local": true, "keycloak": true})
}

func (a *platformAuth) callback(w http.ResponseWriter, r *http.Request) {
	state, err := r.Cookie("orkest_oauth_state")
	nonce, nonceErr := r.Cookie("orkest_oauth_nonce")
	if err != nil || nonceErr != nil || state.Value == "" || nonce.Value == "" || r.URL.Query().Get("state") != state.Value {
		http.Error(w, "invalid login state", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "orkest_oauth_state", Value: "", Path: "/auth", HttpOnly: true, Secure: a.secure, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "orkest_oauth_nonce", Value: "", Path: "/auth", HttpOnly: true, Secure: a.secure, MaxAge: -1})
	ctx := context.WithValue(r.Context(), oauth2.HTTPClient, a.httpClient)
	token, err := a.config.Exchange(ctx, r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	idToken, err := a.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	var tokenClaims struct {
		Nonce         string `json:"nonce"`
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Username      string `json:"preferred_username"`
	}
	if err := idToken.Claims(&tokenClaims); err != nil || tokenClaims.Nonce != nonce.Value || tokenClaims.Subject == "" {
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	if tokenClaims.EmailVerified {
		_, err = a.local.LinkKeycloakIdentity(r.Context(), tokenClaims.Subject, tokenClaims.Email, true)
		if errors.Is(err, identity.ErrNoLocalAccountToLink) {
			_, err = a.users.EnsureUserProfile(r.Context(), tokenClaims.Subject, tokenClaims.Username, tokenClaims.Email)
		}
	} else {
		var hasLocalAccount bool
		hasLocalAccount, err = a.local.HasAccountEmail(r.Context(), tokenClaims.Email)
		if err == nil && hasLocalAccount {
			err = identity.ErrInvalidCredentials
		} else if err == nil {
			_, err = a.users.EnsureUserProfile(r.Context(), tokenClaims.Subject, tokenClaims.Username, tokenClaims.Email)
		}
	}
	if err != nil {
		logServerError(r, "register authenticated user", err)
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	maxAge := int(time.Until(idToken.Expiry).Seconds())
	if maxAge < 1 {
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: rawIDToken, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge, Expires: idToken.Expiry})
	if token.RefreshToken != "" {
		http.SetCookie(w, &http.Cookie{Name: refreshCookie, Value: token.RefreshToken, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 60 * 60})
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *platformAuth) logout(w http.ResponseWriter, r *http.Request) {
	localToken := ""
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer local.") {
		localToken = strings.TrimPrefix(header, "Bearer ")
	} else if cookie, err := r.Cookie(sessionCookie); err == nil {
		localToken = cookie.Value
	}
	if localToken != "" {
		if err := a.local.RevokeSession(r.Context(), localToken); err != nil {
			logServerError(r, "revoke local session", err)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: refreshCookie, Value: "", Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (a *platformAuth) localLogin(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &request) {
		return
	}
	client := r.RemoteAddr
	if host, _, err := net.SplitHostPort(client); err == nil {
		client = host
	}
	if !a.allowLocalLogin(client, time.Now()) {
		writeError(w, http.StatusTooManyRequests, "login_rate_limited")
		return
	}
	_, token, err := a.local.Authenticate(r.Context(), request.Username, request.Password)
	if err != nil {
		if errors.Is(err, identity.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials")
		} else {
			logServerError(r, "authenticate local account", err)
			writeError(w, http.StatusInternalServerError, "internal_error")
		}
		return
	}
	a.clearLocalLoginFailures(client)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: 12 * 60 * 60, Expires: time.Now().Add(12 * time.Hour)})
	w.WriteHeader(http.StatusNoContent)
}

func (a *platformAuth) allowLocalLogin(client string, now time.Time) bool {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	if a.loginFailures == nil {
		a.loginFailures = make(map[string][]time.Time)
	}
	a.pruneLocalLoginFailures(now)
	if len(a.loginFailures[client]) >= 10 {
		return false
	}
	if _, exists := a.loginFailures[client]; !exists && len(a.loginFailures) >= 10_000 {
		return false
	}
	a.loginFailures[client] = append(a.loginFailures[client], now)
	return true
}

func (a *platformAuth) clearLocalLoginFailures(client string) {
	a.loginMu.Lock()
	delete(a.loginFailures, client)
	a.loginMu.Unlock()
}

func (a *platformAuth) pruneLocalLoginFailures(now time.Time) {
	for client, failures := range a.loginFailures {
		kept := failures[:0]
		for _, at := range failures {
			if now.Sub(at) < 15*time.Minute {
				kept = append(kept, at)
			}
		}
		if len(kept) == 0 {
			delete(a.loginFailures, client)
		} else {
			a.loginFailures[client] = kept
		}
	}
}

func (a *platformAuth) currentUser(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userContextKey{}).(identitymodel.User)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	organizations, err := a.organizations.ListOrganizations(r.Context(), user.ID)
	if err != nil {
		logServerError(r, "load current user organizations", err)
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	platformAdmin, err := a.users.IsPlatformAdmin(r.Context(), user.ID)
	if err != nil {
		logServerError(r, "load current user permissions", err)
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	hasLocalPassword := false
	if a.local != nil {
		hasLocalPassword, err = a.local.HasLocalPassword(r.Context(), user.ID)
		if err != nil {
			logServerError(r, "load current user authentication settings", err)
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		PlatformAdmin    bool                             `json:"platformAdmin"`
		HasLocalPassword bool                             `json:"hasLocalPassword"`
		User             identitymodel.User               `json:"user"`
		Organizations    []organizationmodel.Organization `json:"organizations"`
	}{platformAdmin, hasLocalPassword, user, organizations})
}

func (a *platformAuth) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := ""
		if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
			raw = strings.TrimPrefix(header, "Bearer ")
		} else if cookie, err := r.Cookie(sessionCookie); err == nil {
			raw = cookie.Value
		}
		if strings.HasPrefix(raw, "local.") {
			user, err := a.local.UserForSession(r.Context(), raw)
			if err != nil {
				if errors.Is(err, identity.ErrInvalidCredentials) {
					writeError(w, http.StatusUnauthorized, "unauthorized")
				} else {
					logServerError(r, "load local session", err)
					writeError(w, http.StatusInternalServerError, "internal_error")
				}
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
			return
		}
		var token *oidc.IDToken
		var err error
		if raw != "" {
			token, err = a.verifier.Verify(r.Context(), raw)
		}
		if shouldRefreshSession(raw, r.Header.Get("Authorization"), err) {
			if refresh, cookieErr := r.Cookie(refreshCookie); cookieErr == nil && refresh.Value != "" {
				refreshed, refreshErr := a.refreshSession(r.Context(), refresh.Value)
				if refreshErr == nil {
					if refreshedIDToken, ok := refreshed.Extra("id_token").(string); ok && refreshedIDToken != "" {
						raw = refreshedIDToken
						token, err = a.verifier.Verify(r.Context(), raw)
						if err == nil {
							setSessionCookies(w, a.secure, raw, refreshed.RefreshToken, token.Expiry)
						}
					}
				}
			}
		}
		if token == nil || err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var claims struct {
			Subject  string `json:"sub"`
			Email    string `json:"email"`
			Username string `json:"preferred_username"`
		}
		if err := token.Claims(&claims); err != nil || claims.Subject == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		user, err := a.users.EnsureUserProfile(r.Context(), claims.Subject, claims.Username, claims.Email)
		if err != nil {
			logServerError(r, "register authenticated user", err)
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	})
}

func shouldRefreshSession(raw, authorization string, verifyErr error) bool {
	return authorization == "" && (raw == "" || verifyErr != nil)
}

func (a *platformAuth) refreshSession(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, a.httpClient)
	// x/oauth2's refresh token source omits scope. Keycloak only returns a new
	// ID token when the refresh grant explicitly asks for the OpenID scope.
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {a.config.ClientID},
		"client_secret": {a.config.ClientSecret},
		"scope":         {strings.Join(a.config.Scopes, " ")},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.Endpoint.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create OIDC refresh request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := a.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send OIDC refresh request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil, fmt.Errorf("OIDC refresh returned status %d", response.StatusCode)
	}
	var fields map[string]any
	if err := json.NewDecoder(response.Body).Decode(&fields); err != nil {
		return nil, fmt.Errorf("decode OIDC refresh response: %w", err)
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode OIDC refresh response: %w", err)
	}
	var token oauth2.Token
	if err := json.Unmarshal(encoded, &token); err != nil {
		return nil, fmt.Errorf("decode OIDC refresh token: %w", err)
	}
	if token.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}
	if token.RefreshToken == "" {
		token.RefreshToken = refreshToken
	}
	return token.WithExtra(fields), nil
}

func setSessionCookies(w http.ResponseWriter, secure bool, idToken, refreshToken string, expires time.Time) {
	maxAge := int(time.Until(expires).Seconds())
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: idToken, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge, Expires: expires})
	if refreshToken != "" {
		http.SetCookie(w, &http.Cookie{Name: refreshCookie, Value: refreshToken, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 60 * 60})
	}
}

func logServerError(r *http.Request, message string, err error) {
	logger := httplog.LoggerFromContext(r.Context())
	if logger == nil {
		logger = slog.Default()
	}
	logger.Error(message, "error", err)
}

func currentUserID(r *http.Request) string {
	user, _ := r.Context().Value(userContextKey{}).(identitymodel.User)
	return user.ID
}

func randomState() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
