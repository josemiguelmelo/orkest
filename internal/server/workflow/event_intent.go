package workflow

import "fmt"

// EventIntent is a complete business event identity, independent of SQL rows.
// Scope and actor are supplied by the enclosing locked transaction.
type EventIntent struct {
	ID                                string
	StageVersion                      int64
	Type                              EventType
	RunID, CausationID, CorrelationID string
}

func StageEvent(taskID string, version int64, kind EventType, runID, cause, correlation string) EventIntent {
	id := fmt.Sprintf("%s:%d:%s", taskID, version, kind)
	if kind == StageCompleted {
		id = fmt.Sprintf("%s:%d:completed:%s", taskID, version, cause)
		// Historical explicit completion commands retain empty causation when callers
		// omit a command ID; do not replace that identity with stage-entry defaults.
		return EventIntent{ID: id, StageVersion: version, Type: kind, RunID: runID, CausationID: cause, CorrelationID: correlation}
	}
	if cause == "" {
		cause = id
	}
	if correlation == "" {
		correlation = cause
	}
	return EventIntent{ID: id, StageVersion: version, Type: kind, RunID: runID, CausationID: cause, CorrelationID: correlation}
}
