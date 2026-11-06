package taskintegration

import (
	"time"
)

const (
	DefaultInterval = 5 * time.Minute
)

type ValidationError struct {
	Code  string
	Cause error
}

func (e *ValidationError) Error() string {
	if e.Code != "" {
		return e.Code
	}
	return "integration connectivity validation failed"
}

func (e *ValidationError) Unwrap() error { return e.Cause }

type ConfigurationError struct{ Code string }

func (e *ConfigurationError) Error() string { return e.Code }
