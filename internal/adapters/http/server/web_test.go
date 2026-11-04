package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestWebRouting(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"index.html": "<html>Orkest SPA</html>", "app.js": "console.log('asset')"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	router := chi.NewRouter()
	router.Get("/api/me", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	mountWeb(router, dir)
	for _, tt := range []struct {
		method, path string
		status       int
		body         string
	}{
		{"GET", "/", 200, "Orkest SPA"},
		{"GET", "/organizations/org/workspaces/ws/tasks", 200, "Orkest SPA"},
		{"GET", "/app.js", 200, "console.log"},
		{"HEAD", "/organizations/org", 200, ""},
		{"GET", "/missing.js", 404, ""},
		{"GET", "/assets/missing", 404, ""},
		{"GET", "/api/missing", 404, ""},
		{"GET", "/auth/missing", 404, ""},
		{"GET", "/docs/missing", 404, ""},
		{"GET", "/healthz", 404, ""},
		{"GET", "/api/me", 401, ""},
		{"POST", "/organizations/org", 404, ""},
	} {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			r := httptest.NewRecorder()
			router.ServeHTTP(r, httptest.NewRequest(tt.method, tt.path, nil))
			if r.Code != tt.status || !strings.Contains(r.Body.String(), tt.body) {
				t.Fatalf("got %d %q", r.Code, r.Body.String())
			}
		})
	}
}

func TestWebMissingBuild(t *testing.T) {
	router := chi.NewRouter()
	mountWeb(router, t.TempDir())
	r := httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/", nil))
	if r.Code != http.StatusNotFound {
		t.Fatalf("got %d", r.Code)
	}
}
