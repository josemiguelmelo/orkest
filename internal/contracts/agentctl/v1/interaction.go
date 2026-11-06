package agentctlv1

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

type PermissionResponse struct {
	TaskID    string `json:"taskId"`
	RunID     string `json:"runId"`
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	OptionID  string `json:"optionId"`
}

type PromptRequest struct {
	CommandID string `json:"commandId"`
	Text      string `json:"text"`
}

type (
	CancelRequest struct {
		CommandID string `json:"commandId"`
	}
	CommandResponse struct {
		Status    string `json:"status"`
		CommandID string `json:"commandId,omitempty"`
		RequestID string `json:"requestId,omitempty"`
	}
)

type CommandStatus struct {
	CommandID string `json:"commandId"`
	Status    string `json:"status"`
}
