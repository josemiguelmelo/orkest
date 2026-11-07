package runtime

import (
	"context"
	"io"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
)

// ProtocolClientFactory only binds an ACP-compatible protocol implementation
// to agentctl-owned streams. It cannot start processes or provision the runtime.
type ProtocolClientFactory interface {
	Connect(context.Context, io.WriteCloser, io.ReadCloser, ProtocolCallbacks) (ProtocolClient, error)
}

type ProtocolCallbacks interface {
	Update(interaction.Update) error
	Permission(context.Context, interaction.Permission) (string, error)
	Observation(HistoryObservation)
}

// QuestionCallbacks is implemented by hosts that can durably handle input forms.
type QuestionCallbacks interface {
	Question(context.Context, interaction.Question) (interaction.QuestionAnswer, error)
}

type NegotiatedCapabilities struct {
	ProtocolVersion int
	LoadSession     bool
	SetSessionMode  bool
}

// ProtocolModelSelector must confirm the exact model or return an error.
type ProtocolModelSelector interface {
	SetSessionModel(context.Context, string, string) error
}

type ProtocolClient interface {
	ProtocolModelSelector
	Initialize(context.Context) (NegotiatedCapabilities, error)
	NewSession(context.Context, string, ProtocolSessionOptions) (string, error)
	// LoadSession returns only after all replay callbacks preceding its response
	// have completed. The host seals recovery evidence at this boundary.
	LoadSession(context.Context, string, string, ProtocolSessionOptions) error
	SetSessionMode(context.Context, string, string) error
	Prompt(context.Context, string, string) (StopReason, error)
	Cancel(context.Context, string) error
	Done() <-chan struct{}
	Err() error
}

type HistoryObservationKind string

const (
	HistoryUserChunk    HistoryObservationKind = "user_chunk"
	HistoryToolActivity HistoryObservationKind = "tool_activity"
	HistoryReplayDone   HistoryObservationKind = "replay_done"
	HistoryUnknown      HistoryObservationKind = "unknown"
	HistoryIncomplete   HistoryObservationKind = "incomplete"
)

type HistoryObservation struct {
	Kind HistoryObservationKind
	Text string
}
