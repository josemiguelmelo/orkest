package agentctl

// State describes execution progress independently of process liveness.
type State string

const (
	StateStarting          State = "starting"
	StateReady             State = "ready"
	StateRunning           State = "running"
	StateWaitingPermission State = "waiting_for_permission"
	StateFailed            State = "failed"
	StateInterrupted       State = "interrupted"
	StateExited            State = "exited"
)
