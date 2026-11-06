package orchestration

// AgentctlFailure exposes classified, sanitized worker diagnostics to policy.
// Cause retains transport identity for callers without exposing it in task content.
type AgentctlFailure struct {
	Code    string
	Message string
	Cause   error
}

func (e *AgentctlFailure) Error() string {
	if e.Code == "launch_failed" && e.Message != "" {
		return "agentctl launch failed: " + e.Message
	}
	return "agentctl request failed"
}
func (e *AgentctlFailure) Unwrap() error { return e.Cause }
