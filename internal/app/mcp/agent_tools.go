package mcp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/coderepository"
	"github.com/josemiguelmelo/orkest/internal/mcpserver"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const agentToolsPath = "/mcp/agent-tools"

type AgentTools struct {
	key          []byte
	endpoint     string
	repositories interface {
		List(context.Context, string, string, string) ([]coderepository.CodeRepository, error)
	}
	tasks interface {
		Get(context.Context, string, string, string, string) (task.Task, error)
	}
}

type agentClaims struct {
	UserID      string `json:"u"`
	OrgID       string `json:"o"`
	WorkspaceID string `json:"w"`
	TaskID      string `json:"t"`
	Expires     int64  `json:"e"`
}

type listWorkspaceRepositoriesOutput struct {
	Repositories []coderepository.CodeRepository `json:"repositories"`
}

func NewAgentTools(key, endpoint string, repositories interface {
	List(context.Context, string, string, string) ([]coderepository.CodeRepository, error)
}, tasks interface {
	Get(context.Context, string, string, string, string) (task.Task, error)
},
) (*AgentTools, error) {
	key = strings.TrimSpace(key)
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	parsedEndpoint, endpointErr := url.Parse(endpoint)
	if len(key) < 32 || endpointErr != nil || parsedEndpoint == nil || (parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https") || parsedEndpoint.Host == "" || parsedEndpoint.User != nil || repositories == nil || tasks == nil {
		return nil, errors.New("agent MCP tools require an endpoint, a 32-byte signing key, and read services")
	}
	return &AgentTools{key: []byte(key), endpoint: endpoint + agentToolsPath, repositories: repositories, tasks: tasks}, nil
}

// AgentToolsTokenKey uses an explicitly configured key or a durable local key.
func AgentToolsTokenKey(dataRoot, configured string) (string, error) {
	if configured = strings.TrimSpace(configured); configured != "" {
		return configured, nil
	}
	path := filepath.Join(dataRoot, "agent-mcp-token.key")
	if value, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(value)), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read agent MCP token key: %w", err)
	}
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate agent MCP token key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		stored, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", readErr
		}
		return strings.TrimSpace(string(stored)), nil
	}
	if err != nil {
		return "", fmt.Errorf("create agent MCP token key: %w", err)
	}
	if _, err = file.WriteString(base64.RawURLEncoding.EncodeToString(value)); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write agent MCP token key: %w", err)
	}
	if err = file.Close(); err != nil {
		return "", fmt.Errorf("close agent MCP token key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (s *AgentTools) RuntimeServer(userID, orgID, workspaceID, taskID string) (mcpserver.RuntimeServer, error) {
	claims := agentClaims{UserID: userID, OrgID: orgID, WorkspaceID: workspaceID, TaskID: taskID, Expires: time.Now().Add(24 * time.Hour).Unix()}
	payload, err := json.Marshal(claims)
	if err != nil {
		return mcpserver.RuntimeServer{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(encoded))
	return mcpserver.RuntimeServer{ID: runconfig.OrkestMCPServerID, Name: "Orkest", URL: s.endpoint, Headers: map[string]string{"Authorization": "Bearer " + encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))}}, nil
}

func (s *AgentTools) Handler() http.Handler {
	protocol := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		claims, err := s.verify(r.Header.Get("Authorization"))
		if err != nil {
			return nil
		}
		server := mcp.NewServer(&mcp.Implementation{Name: "orkest", Version: "1.0.0"}, nil)
		mcp.AddTool(server, &mcp.Tool{Name: "list_workspace_repositories", Description: "List repositories registered in this task's workspace."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listWorkspaceRepositoriesOutput, error) {
			value, callErr := s.repositories.List(ctx, claims.UserID, claims.OrgID, claims.WorkspaceID)
			if callErr != nil {
				return nil, listWorkspaceRepositoriesOutput{}, errors.New("unable to list workspace repositories")
			}
			return nil, listWorkspaceRepositoriesOutput{Repositories: value}, nil
		})
		type getTaskInput struct {
			TaskID string `json:"task_id" jsonschema:"Task ID to retrieve; defaults to the current task."`
		}
		mcp.AddTool(server, &mcp.Tool{Name: "get_task", Description: "Get task details. The current task is returned by default; other tasks must belong to the same workspace."}, func(ctx context.Context, _ *mcp.CallToolRequest, input getTaskInput) (*mcp.CallToolResult, any, error) {
			id := strings.TrimSpace(input.TaskID)
			if id == "" {
				id = claims.TaskID
			}
			value, callErr := s.tasks.Get(ctx, claims.UserID, claims.OrgID, claims.WorkspaceID, id)
			if callErr != nil {
				return nil, nil, errors.New("unable to get task")
			}
			return nil, value, nil
		})
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := s.verify(r.Header.Get("Authorization")); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		protocol.ServeHTTP(w, r)
	})
}

func (s *AgentTools) verify(header string) (agentClaims, error) {
	var claims agentClaims
	parts := strings.Split(strings.TrimPrefix(header, "Bearer "), ".")
	if len(parts) != 2 {
		return claims, errors.New("invalid agent MCP authorization")
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, errors.New("invalid agent MCP authorization")
	}
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return claims, errors.New("invalid agent MCP authorization")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(payload, &claims) != nil || claims.UserID == "" || claims.OrgID == "" || claims.WorkspaceID == "" || claims.TaskID == "" || claims.Expires < time.Now().Unix() {
		return agentClaims{}, errors.New("invalid or expired agent MCP authorization")
	}
	return claims, nil
}
