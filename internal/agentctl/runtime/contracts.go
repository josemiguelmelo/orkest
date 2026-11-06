// Package runtime defines normalized host, session, profile, and protocol contracts.
package runtime

import (
	"context"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

// RuntimeHost owns process startup, durable identity, and runtime session setup.
// Concrete implementations live in runtimehost and receive profiles and protocol
// communication factories through constructor injection.
// RuntimeHost owns exactly one runtime assigned to this agentctl worker.
type RuntimeHost interface {
	Runtime() coreruntime.Runtime
	Version() string
	Launch(context.Context, configmodel.RuntimeConfig) (RuntimeSession, error)
	Restore(context.Context, configmodel.RuntimeConfig) (RuntimeSession, error)
}

type RuntimeSession interface {
	SessionID() string
	Done() <-chan struct{}
	Err() error
	Close() error
	Prompt(context.Context, interaction.Command, PromptObserver) (StopReason, error)
	Cancel(context.Context) error
	Reconcile(context.Context, interaction.Command) (RecoveryDecision, error)
}

// StopReason is the normalized reason a runtime ended a prompt. Protocol
// adapters preserve additional reasons and normalize user cancellation.
type StopReason string

const (
	StopReasonEndTurn   StopReason = "end_turn"
	StopReasonCancelled StopReason = "cancelled"
)

type RecoveryAction string

const (
	RecoveryContinue RecoveryAction = "continue_safe"
	RecoveryPause    RecoveryAction = "requires_reconciliation"
)

type RecoveryDecision struct {
	Action RecoveryAction
	Reason string
}

type PromptObserver struct {
	Update     func(interaction.Update) error
	Permission func(context.Context, interaction.Permission) (string, error)
}
