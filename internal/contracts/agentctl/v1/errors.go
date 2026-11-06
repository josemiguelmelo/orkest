package agentctlv1

type Error struct {
	Code    string `json:"errorCode"`
	Message string `json:"error"`
	Field   string `json:"field,omitempty"`
}
