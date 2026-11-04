package protocol

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	wire "github.com/josemiguelmelo/orkest/internal/contracts/agentctl/v1"
)

// ValidationError is the shared protocol representation of a rejected field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func WriteValidationError(w http.ResponseWriter, status int, code string, validation *ValidationError) {
	WriteJSON(w, status, wire.Error{Code: code, Message: validation.Error(), Field: validation.Field})
}

func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, wire.Error{Code: code, Message: message})
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
		WriteError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "application/json required")
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxMessageBytes))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			WriteError(w, http.StatusRequestEntityTooLarge, "body_too_large", "request body exceeds 1 MiB")
		} else {
			WriteError(w, http.StatusBadRequest, "unreadable_body", "could not read request body")
		}
		return false
	}
	if err := json.Unmarshal(body, destination); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON matching the expected format")
		return false
	}
	return true
}
