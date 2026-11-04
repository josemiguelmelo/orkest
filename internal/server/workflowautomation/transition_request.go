package workflowautomation

type TransitionRequest struct {
	StageID      string `json:"stageId"`
	StageVersion int64  `json:"stageVersion"`
	Action       string `json:"action"`
	CommandID    string `json:"commandId"`
	SessionMode  string `json:"sessionMode"`
	PromptText   string `json:"promptText"`
}
