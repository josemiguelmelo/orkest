package httpapi

type message struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId,omitempty"`
	Info      *Info  `json:"info,omitempty"`
	Error     string `json:"error,omitempty"`
}
