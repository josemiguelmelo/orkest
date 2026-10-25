package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/core/openapi"
)

// NewRouter validates the served OpenAPI contract before exposing routes.
func NewRouter() (*chi.Mux, error) {
	if _, err := openapi.LoadAndValidateOpenAPIDoc(OpenAPIContract, context.Background()); err != nil {
		return nil, fmt.Errorf("load platform OpenAPI contract: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	auth, err := newOIDCAuth(ctx)
	if err != nil {
		return nil, err
	}

	router := chi.NewRouter()
	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	router.Get("/auth/login", auth.login)
	router.Get("/auth/callback", auth.callback)
	router.Post("/auth/logout", auth.logout)
	router.Get("/api/me", auth.currentUser)
	openapi.Enable(router, "Orkest API", OpenAPIContract)
	return router, nil
}
