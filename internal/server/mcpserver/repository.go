package mcpserver

import (
	"context"

	mcpservermodel "github.com/josemiguelmelo/orkest/internal/server/mcpserver/model"
)

type MCPServerRepository interface {
	Create(context.Context, mcpservermodel.Server, string) (mcpservermodel.Server, error)
	Update(context.Context, mcpservermodel.Server, string) (mcpservermodel.Server, error)
	Delete(context.Context, string, string) error
	Get(context.Context, string, string) (mcpservermodel.Server, string, error)
	List(context.Context, string) ([]mcpservermodel.Server, error)
}
