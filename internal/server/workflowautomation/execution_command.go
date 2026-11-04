package workflowautomation

type ExecutionCommand struct {
	ID, OrganizationID, WorkspaceID, TaskID, ActorID string
	StageVersion                                     int64
	Options                                          StartWorkOptions
}

// StartWorkOptions carries execution options only; task transitions and domain
// events have separate payloads.
type StartWorkOptions struct {
	Action       string `json:"action"`
	CommandID    string `json:"commandId"`
	StageVersion int64  `json:"stageVersion"`
	SessionMode  string `json:"sessionMode"`
	PromptText   string `json:"promptText"`
}
