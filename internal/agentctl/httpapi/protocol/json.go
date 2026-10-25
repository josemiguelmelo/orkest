package protocol

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
)

func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

const maxMessageBytes = 1024 * 1024

// ReadJSON decodes a request into destination, which must be a non-nil pointer.
// It writes an HTTP error and returns false when the request cannot be decoded.
func ReadJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		WriteError(w, http.StatusUnsupportedMediaType, "application/json required")
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxMessageBytes))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			WriteError(w, http.StatusRequestEntityTooLarge, "request body exceeds 1 MiB")
		} else {
			WriteError(w, http.StatusBadRequest, "could not read request body")
		}
		return false
	}
	if err := json.Unmarshal(body, destination); err != nil {
		WriteError(w, http.StatusBadRequest, "request body must be valid JSON matching the expected format")
		return false
	}
	return true
}
