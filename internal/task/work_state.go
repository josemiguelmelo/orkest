package task

// WorkState describes the execution state of a task's current workflow stage.
type WorkState string

const (
	WorkStateIdle      WorkState = "idle"
	WorkStateQueued    WorkState = "queued"
	WorkStateRunning   WorkState = "running"
	WorkStateCompleted WorkState = "completed"
	WorkStateFailed    WorkState = "failed"
)
