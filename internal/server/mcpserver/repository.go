package mcpserver

import "context"

type MCPServerRepository interface {
	Create(context.Context, string, string, string, string, string) (Server, error)
	Update(context.Context, string, string, string, string, string) (Server, error)
	Delete(context.Context, string, string) error
	Get(context.Context, string, string) (Server, string, error)
	List(context.Context, string) ([]Server, error)
}
