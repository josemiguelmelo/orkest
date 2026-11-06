package task

type DispatchError struct{ Cause error }

func (e *DispatchError) Error() string { return e.Cause.Error() }

func (e *DispatchError) Unwrap() error { return e.Cause }
