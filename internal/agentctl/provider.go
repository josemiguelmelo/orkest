package agentctl

import "context"

// Provider owns runtime-specific startup. No ACP types cross this boundary.
type Provider interface {
	Runtime() string
	Launch(context.Context, Configuration) (Process, error)
}

// Process is owned by agentctl, never by an HTTP request or stream.
type Process interface {
	SessionID() string
	Done() <-chan struct{}
	Close() error
}
