package openapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/swaggest/swgui/v5emb"
)

func Enable(mux *chi.Mux, title string, openapiData []byte) {
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(openapiData)
	})
	mux.Handle("/docs/*", v5emb.New(title, "/openapi.yaml", "/docs/"))
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusPermanentRedirect)
	})
}
