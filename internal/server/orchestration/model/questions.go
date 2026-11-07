package model

import "encoding/json"

// Question is a form requested by the active runtime invocation.
type Question struct {
	ToolCallID string          `json:"toolCallId,omitempty"`
	Message    string          `json:"message"`
	Schema     json.RawMessage `json:"schema"`
}

// QuestionAnswer is recorded before the waiting invocation is notified.
type QuestionAnswer struct {
	Action  string          `json:"action"`
	Content json.RawMessage `json:"content,omitempty"`
}

type QuestionResponse struct {
	AnsweredBy    string         `json:"answeredBy,omitempty"`
	TaskID        string         `json:"taskId"`
	TaskSessionID string         `json:"taskSessionId"`
	RunID         string         `json:"runId"`
	SessionID     string         `json:"sessionId"`
	CommandID     string         `json:"commandId"`
	RequestID     string         `json:"requestId"`
	SubmissionID  string         `json:"submissionId"`
	Answer        QuestionAnswer `json:"answer"`
}
