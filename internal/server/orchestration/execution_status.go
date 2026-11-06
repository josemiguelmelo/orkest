package orchestration

// ExecutionState describes execution progress independently of process liveness.
type ExecutionState string

const (
	StateStarting          ExecutionState = "starting"
	StateReady             ExecutionState = "ready"
	StateRunning           ExecutionState = "running"
	StateWaitingPermission ExecutionState = "waiting_for_permission"
	StateFailed            ExecutionState = "failed"
	StateInterrupted       ExecutionState = "interrupted"
	StateExited            ExecutionState = "exited"
)

type RunStatus struct {
	OrganizationID     string
	WorkspaceID        string
	TaskID             string
	RunID              string
	RecoveryBlocked    bool
	Configured         bool
	ExecutionAvailable bool
	State              ExecutionState
	ProcessAlive       bool
	SessionID          string
	TaskSessionID      string
	ActiveCommandID    string
	QueueDepth         int
	QueuePaused        bool
	RunEnded           bool
	RunCancelled       bool
	RecoveryPermitted  bool
}
