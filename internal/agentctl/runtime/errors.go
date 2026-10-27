package runtime

import "errors"

type RuntimeFailureKind string

const (
	RuntimeFailure     RuntimeFailureKind = "failure"
	RuntimeInterrupted RuntimeFailureKind = "interrupted"
)

// RuntimeError exposes only a sanitized message. Causes remain available for
// diagnosis through errors.Is/As and must never be printed in ordinary logs.
type RuntimeError struct {
	Kind    RuntimeFailureKind
	Message string
	Cause   error
}

func (e *RuntimeError) Error() string { return e.Message }
func (e *RuntimeError) Unwrap() error { return e.Cause }
func IsRuntimeInterrupted(err error) bool {
	var runtime *RuntimeError
	return errors.As(err, &runtime) && runtime.Kind == RuntimeInterrupted
}

var ErrRuntimeHostUnavailable = errors.New("agent provider unavailable")

// LaunchError carries a provider-sanitized message suitable for API responses.
// Arbitrary provider errors remain internal and are mapped to a generic response.
type LaunchError struct {
	Message string
	Cause   error
}

func (e *LaunchError) Error() string { return e.Message }
func (e *LaunchError) Unwrap() error { return e.Cause }
