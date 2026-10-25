package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	contract "github.com/josemiguelmelo/orkest/api"
	"github.com/swaggest/swgui/v5emb"
)

// NewRouter validates the served OpenAPI contract before exposing routes.
func NewRouter() (http.Handler, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromData(contract.OpenAPI)
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI contract: %w", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("validate OpenAPI contract: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(contract.OpenAPI)
	})
	mux.Handle("GET /docs/", v5emb.New("Orkest API", "/openapi.yaml", "/docs/"))
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusPermanentRedirect)
	})
	return mux, nil
}
