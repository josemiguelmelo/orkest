package httpapi

import _ "embed"

// OpenAPIContract is the platform and orchestrator API contract.
//
//go:embed openapi.yaml
var OpenAPIContract []byte
