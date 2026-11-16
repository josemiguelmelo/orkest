package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/server/identity"
)

type accountAPI struct {
	accounts *identity.LocalAuthenticationService
	users    *identity.UserRegistrationService
}

func (a accountAPI) provision(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !decodeBody(w, r, &request) {
		return
	}
	user, err := a.accounts.Provision(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), request.Username, request.Email, request.Password, request.Role)
	respond(w, user, err, http.StatusCreated)
}

func (a accountAPI) promotePlatformAdmin(w http.ResponseWriter, r *http.Request) {
	err := a.users.PromotePlatformAdmin(r.Context(), currentUserID(r), chi.URLParam(r, "userID"))
	respond(w, nil, err, http.StatusNoContent)
}

func (a accountAPI) changePassword(w http.ResponseWriter, r *http.Request) {
	var request struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decodeBody(w, r, &request) {
		return
	}
	err := a.accounts.ChangePassword(r.Context(), currentUserID(r), request.CurrentPassword, request.NewPassword)
	switch {
	case errors.Is(err, identity.ErrInvalidCredentials):
		writeError(w, http.StatusBadRequest, "current_password_invalid")
	case errors.Is(err, identity.ErrNoLocalPassword):
		writeError(w, http.StatusConflict, "local_password_unavailable")
	default:
		respond(w, nil, err, http.StatusNoContent)
	}
}
