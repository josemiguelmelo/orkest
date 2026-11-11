package model

import "github.com/josemiguelmelo/orkest/internal/contracts/mcptransport"

type Server struct {
	Transport      mcptransport.Transport `json:"transport"`
	Command        string                 `json:"command,omitempty"`
	Args           []string               `json:"args,omitempty"`
	HasEnvironment bool                   `json:"hasEnvironment"`
	ID             string                 `json:"id"`
	WorkspaceID    string                 `json:"workspaceId"`
	Name           string                 `json:"name"`
	URL            string                 `json:"url"`
	HasHeaders     bool                   `json:"hasHeaders"`
}

type Input struct {
	Transport   mcptransport.Transport `json:"transport"`
	Command     string                 `json:"command,omitempty"`
	Args        []string               `json:"args,omitempty"`
	Environment map[string]string      `json:"environment,omitempty"`
	Name        string                 `json:"name"`
	URL         string                 `json:"url"`
	Headers     map[string]string      `json:"headers,omitempty"`
}

// RuntimeServer contains credentials and must only be passed to agentctl.
type RuntimeServer struct {
	Transport   mcptransport.Transport `json:"transport"`
	Command     string                 `json:"command,omitempty"`
	Args        []string               `json:"args,omitempty"`
	Environment map[string]string      `json:"environment,omitempty"`
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	URL         string                 `json:"url"`
	Headers     map[string]string      `json:"headers,omitempty"`
}
