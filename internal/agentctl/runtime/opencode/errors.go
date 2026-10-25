package opencode

// runtimeError retains diagnostics for errors.Is/As without including runtime
// output, paths, credentials, or protocol payloads in ordinary error strings.
type runtimeError struct {
	message string
	cause   error
}

func (e *runtimeError) Error() string { return e.message }
func (e *runtimeError) Unwrap() error { return e.cause }

func runtimeFailure(message string, cause error) error {
	return &runtimeError{message: message, cause: cause}
}
