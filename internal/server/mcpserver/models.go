package mcpserver

type Server struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	HasHeaders  bool   `json:"hasHeaders"`
}

type Input struct {
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// RuntimeServer contains credentials and must only be passed to agentctl.
type RuntimeServer struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}
