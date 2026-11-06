package httpapi

import (
	"context"
	"testing"

	coreopenapi "github.com/josemiguelmelo/orkest/internal/platform/openapi"
)

func TestPlatformOpenAPIContractValid(t *testing.T) {
	if _, err := coreopenapi.LoadAndValidateOpenAPIDoc(OpenAPIContract, context.Background()); err != nil {
		t.Fatalf("platform OpenAPI contract invalid: %v", err)
	}
}
