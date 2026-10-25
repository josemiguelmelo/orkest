package agentctl

import "context"

import coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"

// Provider owns runtime-specific startup. No ACP types cross this boundary.
type RuntimeProvider interface {
	Runtime() coreruntime.Runtime
	Launch(context.Context, RunConfig) (Process, error)
}

// Process is owned by agentctl, never by an HTTP request or stream.
type Process interface {
	SessionID() string
	Done() <-chan struct{}
	// Err returns the sanitized exit error after Done closes, or nil while running.
	// Wrapped causes are for internal diagnosis and must not be logged directly.
	Err() error
	Close() error
}

// RuntimeConversation is an optional process capability. ACP details remain in adapters.
type RuntimeConversation interface {
	Prompt(context.Context, string, Observer) (string, error)
	Cancel(context.Context) error
}

type Update struct {
	Kind       string `json:"kind"`
	Text       string `json:"text,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	Title      string `json:"title,omitempty"`
	Status     string `json:"status,omitempty"`
}

type PermissionOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type Permission struct {
	ToolCallID string             `json:"toolCallId"`
	Title      string             `json:"title"`
	Options    []PermissionOption `json:"options"`
}

type Observer struct {
	Update     func(Update) error
	Permission func(context.Context, Permission) (string, error)
}
