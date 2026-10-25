package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const sessionCookie = "orkest_session"

type oidcAuth struct {
	config   oauth2.Config
	verifier *oidc.IDTokenVerifier
	secure   bool
}

func newOIDCAuth(ctx context.Context) (*oidcAuth, error) {
	issuer := envOr("OIDC_ISSUER_URL", "http://localhost:8082/realms/orkest")
	clientID := envOr("OIDC_CLIENT_ID", "orkest-web")
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	redirectURL := envOr("OIDC_REDIRECT_URL", "http://localhost:8080/auth/callback")
	return &oidcAuth{
		config: oauth2.Config{
			ClientID: clientID, ClientSecret: envOr("OIDC_CLIENT_SECRET", "orkest-local-secret"),
			Endpoint: provider.Endpoint(), RedirectURL: redirectURL,
			Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
		secure:   strings.EqualFold(os.Getenv("OIDC_COOKIE_SECURE"), "true"),
	}, nil
}

func (a *oidcAuth) login(w http.ResponseWriter, r *http.Request) {
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

func (a *oidcAuth) callback(w http.ResponseWriter, r *http.Request) {
	state, err := r.Cookie("orkest_oauth_state")
	nonce, nonceErr := r.Cookie("orkest_oauth_nonce")
	if err != nil || nonceErr != nil || state.Value == "" || nonce.Value == "" || r.URL.Query().Get("state") != state.Value {
		http.Error(w, "invalid login state", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "orkest_oauth_state", Value: "", Path: "/auth", HttpOnly: true, Secure: a.secure, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "orkest_oauth_nonce", Value: "", Path: "/auth", HttpOnly: true, Secure: a.secure, MaxAge: -1})
	token, err := a.config.Exchange(r.Context(), r.URL.Query().Get("code"))
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
		Nonce string `json:"nonce"`
	}
	if err := idToken.Claims(&tokenClaims); err != nil || tokenClaims.Nonce != nonce.Value {
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	maxAge := int(time.Until(idToken.Expiry).Seconds())
	if maxAge < 1 {
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: rawIDToken, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge, Expires: idToken.Expiry})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *oidcAuth) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (a *oidcAuth) currentUser(w http.ResponseWriter, r *http.Request) {
	raw := ""
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		raw = strings.TrimPrefix(header, "Bearer ")
	} else if cookie, err := r.Cookie(sessionCookie); err == nil {
		raw = cookie.Value
	}
	if raw == "" {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	token, err := a.verifier.Verify(r.Context(), raw)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	var claims struct {
		Subject string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
	}
	if err := token.Claims(&claims); err != nil || claims.Subject == "" {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"sub": claims.Subject, "email": claims.Email, "name": claims.Name})
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
