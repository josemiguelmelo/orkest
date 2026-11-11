package orchestration

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	mcpservermodel "github.com/josemiguelmelo/orkest/internal/server/mcpserver/model"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

func (d *TaskDispatcher) resolveHostMounts(ctx context.Context, config *orchestrationmodel.RunConfig) error {
	if d.mounts == nil {
		return nil
	}
	mounts, err := d.mounts.Resolve(ctx, config.WorkspaceID)
	if err != nil {
		return fmt.Errorf("resolve workspace host mounts: %w", err)
	}
	config.HostMounts = mounts
	return nil
}

func (d *TaskDispatcher) resolveExecutionImage(ctx context.Context, workspaceID string, config *orchestrationmodel.RunConfig) error {
	if config.ExecutionProfileID == "" {
		return nil
	}
	if d.profiles == nil {
		return errors.New("execution profiles unavailable")
	}
	image, err := d.profiles.Resolve(ctx, workspaceID, config.ExecutionProfileID, config.Runtime)
	if err != nil {
		return fmt.Errorf("resolve execution profile: %w", err)
	}
	config.ExecutionImage = image
	return nil
}

func (d *TaskDispatcher) refreshOrkestMCPServer(userID, organizationID, workspaceID, taskID string, config *orchestrationmodel.RunConfig) error {
	if d.agentTools == nil {
		return nil
	}
	server, err := d.agentTools.RuntimeServerForRun(userID, organizationID, workspaceID, taskID, config.RunID)
	if err != nil {
		return fmt.Errorf("configure Orkest agent tools: %w", err)
	}
	replacement := orchestrationmodel.MCPServer{ID: server.ID, Name: server.Name, URL: server.URL, Headers: server.Headers}
	servers := make([]orchestrationmodel.MCPServer, 0, len(config.MCPServers)+1)
	found := false
	for _, existing := range config.MCPServers {
		if existing.ID != server.ID {
			servers = append(servers, existing)
			continue
		}
		if !found {
			servers = append(servers, replacement)
			found = true
		}
	}
	if !found {
		servers = append(servers, replacement)
	}
	config.MCPServers = servers
	return nil
}

// Refresh credentials without replacing the compatible saved server settings.
func refreshSessionMCPHeaders(saved []orchestrationmodel.MCPServer, resolved []mcpservermodel.RuntimeServer) ([]orchestrationmodel.MCPServer, error) {
	byID := make(map[string]mcpservermodel.RuntimeServer, len(resolved))
	for _, server := range resolved {
		if _, exists := byID[server.ID]; exists {
			return nil, errors.New("duplicate resolved MCP server")
		}
		byID[server.ID] = server
	}
	updated := slices.Clone(saved)
	for i := range updated {
		if updated[i].ID == orchestrationmodel.OrkestMCPServerID {
			continue
		}
		server, found := byID[updated[i].ID]
		if !found {
			return nil, errors.New("session MCP server is unavailable")
		}
		savedTransport, currentTransport := updated[i].Transport, server.Transport
		if savedTransport == "" {
			savedTransport = "http"
		}
		if currentTransport == "" {
			currentTransport = "http"
		}
		if savedTransport != currentTransport {
			return nil, errors.New("session MCP transport is incompatible")
		}
		if savedTransport == "stdio" {
			env := make(map[string]string, len(updated[i].Environment))
			for name := range updated[i].Environment {
				value, exists := server.Environment[name]
				if !exists {
					return nil, errors.New("session MCP environment variable is unavailable")
				}
				env[name] = value
			}
			updated[i].Environment = env
		} else {
			updated[i].Headers = maps.Clone(server.Headers)
		}
	}
	return updated, nil
}

func (d *TaskDispatcher) refreshSessionMCPServers(ctx context.Context, workspaceID string, config *orchestrationmodel.RunConfig) error {
	serverIDs := make([]string, 0, len(config.MCPServers))
	for _, server := range config.MCPServers {
		if server.ID != orchestrationmodel.OrkestMCPServerID {
			serverIDs = append(serverIDs, server.ID)
		}
	}
	// Built-in tools have their own run-scoped credentials and require no external resolver.
	if len(serverIDs) == 0 {
		return nil
	}
	if d.mcpServers == nil {
		return errors.New("session MCP server resolver is unavailable")
	}
	servers, err := d.mcpServers.Resolve(ctx, workspaceID, serverIDs)
	if err != nil {
		return fmt.Errorf("resolve session MCP server credentials: %w", err)
	}
	refreshed, err := refreshSessionMCPHeaders(config.MCPServers, servers)
	if err != nil {
		return err
	}
	config.MCPServers = refreshed
	return nil
}
