package model

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
