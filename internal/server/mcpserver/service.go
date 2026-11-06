package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/platform/secretbox"
	"github.com/josemiguelmelo/orkest/internal/server/workspace"
)

var headerNamePattern = regexp.MustCompile(`^[!#$%&'*+.^_\x60|~0-9A-Za-z-]+$`)

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error)
}

type MCPServerService struct {
	repository MCPServerRepository
	box        *secretbox.Box
	workspaces WorkspaceAccess
}

func NewMCPServerService(repository MCPServerRepository, box *secretbox.Box, workspaces WorkspaceAccess) *MCPServerService {
	return &MCPServerService{repository: repository, box: box, workspaces: workspaces}
}

func (s *MCPServerService) List(ctx context.Context, userID, orgID, workspaceID string) ([]Server, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, workspaceID)
}

func (s *MCPServerService) Create(ctx context.Context, userID, orgID, workspaceID string, in Input) (Server, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Server{}, err
	}
	in, err := normalize(in)
	if err != nil {
		return Server{}, err
	}
	sealed, err := s.seal(in.Headers)
	if err != nil {
		return Server{}, err
	}
	id, err := ids.NewUUID()
	if err != nil {
		return Server{}, err
	}
	return s.repository.Create(ctx, id, workspaceID, in.Name, in.URL, string(sealed))
}

func (s *MCPServerService) Update(ctx context.Context, userID, orgID, workspaceID, serverID string, in Input) (Server, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Server{}, err
	}
	old, encrypted, err := s.repository.Get(ctx, workspaceID, serverID)
	if err != nil {
		return Server{}, err
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = old.Name
	}
	if strings.TrimSpace(in.URL) == "" {
		in.URL = old.URL
	}
	in, err = normalize(in)
	if err != nil {
		return Server{}, err
	}
	if in.Headers == nil {
		return s.repository.Update(ctx, workspaceID, serverID, in.Name, in.URL, encrypted)
	}
	sealed, err := s.seal(in.Headers)
	if err != nil {
		return Server{}, err
	}
	return s.repository.Update(ctx, workspaceID, serverID, in.Name, in.URL, string(sealed))
}

func (s *MCPServerService) seal(headers map[string]string) (string, error) {
	if len(headers) == 0 {
		return "", nil
	}
	if s.box == nil {
		return "", secretbox.ErrKeyUnavailable
	}
	encoded, err := json.Marshal(headers)
	if err != nil {
		return "", err
	}
	sealed, err := s.box.Seal(string(encoded))
	if err != nil {
		return "", fmt.Errorf("encrypt MCP headers: %w", err)
	}
	return string(sealed), nil
}

func (s *MCPServerService) Delete(ctx context.Context, userID, orgID, workspaceID, serverID string) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	return s.repository.Delete(ctx, workspaceID, serverID)
}

func (s *MCPServerService) Resolve(ctx context.Context, workspaceID string, serverIDs []string) ([]RuntimeServer, error) {
	result := make([]RuntimeServer, 0, len(serverIDs))
	for _, id := range serverIDs {
		server, encrypted, err := s.repository.Get(ctx, workspaceID, id)
		if err != nil {
			return nil, err
		}
		headers, err := s.decrypt(encrypted)
		if err != nil {
			return nil, err
		}
		result = append(result, RuntimeServer{ID: server.ID, Name: server.Name, URL: server.URL, Headers: headers})
	}
	return result, nil
}

func (s *MCPServerService) Exists(ctx context.Context, _ string, workspaceID string, ids []string) (bool, error) {
	if len(ids) == 0 {
		return true, nil
	}
	servers, err := s.repository.List(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	found := make(map[string]bool, len(servers))
	for _, server := range servers {
		found[server.ID] = true
	}
	for _, id := range ids {
		if !found[id] {
			return false, nil
		}
	}
	return true, nil
}

func (s *MCPServerService) authorize(ctx context.Context, userID, orgID, workspaceID string) error {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role == "viewer" {
		return modelerrors.ErrForbidden
	}
	return nil
}

func (s *MCPServerService) decrypt(encrypted string) (map[string]string, error) {
	if encrypted == "" {
		return map[string]string{}, nil
	}
	if s.box == nil {
		return nil, secretbox.ErrKeyUnavailable
	}
	plain, err := s.box.Open([]byte(encrypted))
	if err != nil {
		return nil, errors.New("decrypt MCP headers")
	}
	var headers map[string]string
	if err = json.Unmarshal([]byte(plain), &headers); err != nil {
		return nil, errors.New("decode MCP headers")
	}
	return headers, nil
}

func normalize(in Input) (Input, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.URL = strings.TrimSpace(in.URL)
	u, err := url.Parse(in.URL)
	if in.Name == "" || len(in.Name) > 200 || err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.Fragment != "" {
		return Input{}, modelerrors.ErrInvalid
	}
	for key, value := range in.Headers {
		if !headerNamePattern.MatchString(key) || strings.ContainsAny(value, "\r\n") || len(key) > 256 || len(value) > 8192 {
			return Input{}, modelerrors.ErrInvalid
		}
	}
	if len(in.Headers) > 50 {
		return Input{}, modelerrors.ErrInvalid
	}
	return in, nil
}
