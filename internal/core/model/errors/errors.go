package errors

import "errors"

var (
	ErrNotFound  = errors.New("resource not found")
	ErrForbidden = errors.New("permission denied")
	ErrConflict  = errors.New("resource conflict")
	ErrInvalid   = errors.New("invalid request")
)
