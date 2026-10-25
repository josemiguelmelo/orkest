package httpapi

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
)

// readJSON decodes a request into destination, which must be a non-nil pointer.
// It writes an HTTP error and returns false when the request cannot be decoded.
func readJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "application/json required")
		return false
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read request body")
		return false
	}
	if err := json.Unmarshal(body, destination); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be valid JSON matching the expected format")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
