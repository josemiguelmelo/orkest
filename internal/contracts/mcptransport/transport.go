// Package mcptransport defines the MCP transports shared by the platform and worker.
package mcptransport

// Transport identifies how a runtime connects to an MCP server.
type Transport string

const (
	HTTP  Transport = "http"
	Stdio Transport = "stdio"
)

// Valid reports whether t is a supported transport. The zero value is invalid.
func (t Transport) Valid() bool {
	return t == HTTP || t == Stdio
}
