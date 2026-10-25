package api

import _ "embed"

// OpenAPI contains the browser API contract served by the platform.
//
//go:embed openapi.yaml
var OpenAPI []byte

// AgentctlOpenAPI contains the standalone agentctl API contract.
//
//go:embed agentctl.openapi.yaml
var AgentctlOpenAPI []byte
