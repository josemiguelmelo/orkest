package openapi

import (
	"context"
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

func LoadAndValidateOpenAPIDoc(openapiData []byte, ctx context.Context) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromData(openapiData)
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI contract: %w", err)
	}
	if err := doc.Validate(ctx); err != nil {
		return nil, fmt.Errorf("validate agentctl OpenAPI contract: %w", err)
	}
	return doc, nil
}
