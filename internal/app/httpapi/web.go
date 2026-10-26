package httpapi

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// mountWeb serves an optional SPA build without masking API or asset errors.
func mountWeb(router chi.Router, directory string) {
	if directory == "" {
		directory = "web/dist"
	}
	files := http.FileServer(http.Dir(directory))
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		for _, prefix := range []string{"/api", "/auth", "/healthz", "/openapi", "/docs", "/swagger"} {
			if r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/") {
				http.NotFound(w, r)
				return
			}
		}
		clean := path.Clean("/" + r.URL.Path)
		info, err := os.Stat(filepath.Join(directory, filepath.FromSlash(strings.TrimPrefix(clean, "/"))))
		if err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		if path.Ext(clean) != "" || strings.HasPrefix(clean, "/assets/") {
			http.NotFound(w, r)
			return
		}
		if _, err := os.Stat(filepath.Join(directory, "index.html")); err != nil {
			if !os.IsNotExist(err) && !os.IsPermission(err) {
				http.Error(w, "web application unavailable", http.StatusServiceUnavailable)
				return
			}
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(directory, "index.html"))
	})
}
