package executionprofile

import "errors"

// BuildError carries a safe operational stage and explanation for logs. Cause is
// retained for control flow but must never be logged or returned to clients.
type BuildError struct {
	Stage   string
	Message string
	Cause   error
}

func (e *BuildError) Error() string { return e.Message }
func (e *BuildError) Unwrap() error { return e.Cause }

func Failure(stage, message string, cause error) error {
	return &BuildError{Stage: stage, Message: message, Cause: cause}
}

func SafeFailure(err error) (stage, message string) {
	var buildErr *BuildError
	if errors.As(err, &buildErr) {
		return buildErr.Stage, buildErr.Message
	}
	return "unknown", "Image operation failed; inspect service and registry connectivity."
}
