package httpapi

import _ "embed"

// OpenAPIContract is the standalone agentctl API contract.
//
//go:embed openapi.yaml
var OpenAPIContract []byte
