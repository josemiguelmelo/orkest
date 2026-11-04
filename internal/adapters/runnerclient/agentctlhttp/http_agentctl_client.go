package agentctlhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/josemiguelmelo/orkest/internal/server/orchestration"

	wire "github.com/josemiguelmelo/orkest/internal/contracts/agentctl/v1"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// HTTPAgentctlClient implements the agentctl REST and activity WebSocket APIs.
type HTTPAgentctlClient struct {
	http      *http.Client
	pollEvery time.Duration
}

type agentctlHTTPError struct {
	Status  int
	Code    string `json:"errorCode"`
	Message string `json:"error"`
}

func (e *agentctlHTTPError) Error() string {
	if e.Code == "launch_failed" && e.Message != "" {
		return fmt.Sprintf("agentctl request returned HTTP %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("agentctl request returned HTTP %d", e.Status)
}

func NewHTTPAgentctlClient(client *http.Client) *HTTPAgentctlClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &HTTPAgentctlClient{http: client, pollEvery: 200 * time.Millisecond}
}

func (c *HTTPAgentctlClient) WaitHealthy(ctx context.Context, endpoint string) error {
	ticker := time.NewTicker(c.pollEvery)
	defer ticker.Stop()
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/health", nil)
		if err == nil {
			resp, requestErr := c.http.Do(req)
			if requestErr == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
				lastErr = fmt.Errorf("health endpoint returned HTTP %d", resp.StatusCode)
			} else {
				lastErr = requestErr
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("agentctl did not become healthy: %w", errors.Join(ctx.Err(), lastErr))
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *HTTPAgentctlClient) Status(ctx context.Context, endpoint string) (orchestration.RunStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/info", nil)
	if err != nil {
		return orchestration.RunStatus{}, fmt.Errorf("create agentctl status request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return orchestration.RunStatus{}, fmt.Errorf("read agentctl status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return orchestration.RunStatus{}, fmt.Errorf("agentctl status endpoint returned HTTP %d", resp.StatusCode)
	}
	var status wire.Info
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&status); err != nil {
		return orchestration.RunStatus{}, fmt.Errorf("decode agentctl status: %w", err)
	}
	return orchestration.RunStatus{
		OrganizationID: status.OrganizationID, WorkspaceID: status.WorkspaceID, TaskID: status.TaskID, RunID: status.RunID,
		RecoveryBlocked: status.RecoveryBlocked, Configured: status.Configured, ExecutionAvailable: status.ExecutionAvailable,
		ProcessAlive: status.ProcessAlive, State: orchestration.ExecutionState(status.State), SessionID: status.SessionID,
		TaskSessionID: status.TaskSessionID, ActiveCommandID: status.ActiveCommandID, QueuePaused: status.QueuePaused, QueueDepth: status.QueueDepth, RunEnded: status.RunEnded,
	}, nil
}

func (c *HTTPAgentctlClient) Configure(ctx context.Context, endpoint string, config orchestration.RunConfig) error {
	return c.post(ctx, endpoint, "/agent/configure", toWireRunConfig(config))
}

func (c *HTTPAgentctlClient) StartAgent(ctx context.Context, endpoint string) error {
	return c.post(ctx, endpoint, "/agent/launch", struct{}{})
}

func (c *HTTPAgentctlClient) Prompt(ctx context.Context, endpoint, commandID, prompt string) error {
	return c.post(ctx, endpoint, "/agent/prompt", wire.PromptRequest{CommandID: commandID, Text: prompt})
}

func (c *HTTPAgentctlClient) CancelPrompt(ctx context.Context, endpoint, commandID string) error {
	return c.post(ctx, endpoint, "/agent/cancel", wire.CancelRequest{CommandID: commandID})
}

func (c *HTTPAgentctlClient) RespondPermission(ctx context.Context, endpoint string, response orchestration.PermissionResponse) error {
	return c.post(ctx, endpoint, "/agent/permissions/respond", toWirePermissionResponse(response))
}

func (c *HTTPAgentctlClient) post(ctx context.Context, endpoint, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode agentctl request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create agentctl request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send agentctl request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var body wire.Error
		if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body); err != nil {
			return &orchestration.AgentctlFailure{Cause: &agentctlHTTPError{Status: resp.StatusCode}}
		}
		response := &agentctlHTTPError{Status: resp.StatusCode, Code: body.Code, Message: body.Message}
		failure := &orchestration.AgentctlFailure{Code: response.Code, Cause: response}
		if response.Code == "launch_failed" {
			failure.Message = response.Message
		}
		return failure
	}
	return nil
}

func (c *HTTPAgentctlClient) Connect(ctx context.Context, endpoint string, cursor uint64, onConnected func(), onEvent func(orchestration.AgentActivityEvent) error) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse agentctl endpoint: %w", err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return errors.New("unsupported agentctl endpoint scheme")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/agent/stream"
	q := u.Query()
	if cursor > 0 {
		q.Set("afterSequence", fmt.Sprintf("%d", cursor))
	}
	u.RawQuery = q.Encode()
	conn, _, err := websocket.Dial(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("connect agentctl stream: %w", err)
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(1 << 20)
	connected := false
	for {
		var frame wire.StreamFrame
		if err = wsjson.Read(ctx, conn, &frame); err != nil {
			return fmt.Errorf("read agentctl stream: %w", err)
		}
		if frame.Type == "ready" && !connected {
			connected = true
			onConnected()
			continue
		}
		if frame.Type == "event" && frame.Event != nil {
			if !connected {
				return errors.New("agentctl sent activity before ready frame")
			}
			if err = onEvent(fromWireEvent(*frame.Event)); err != nil {
				return fmt.Errorf("persist agentctl event: %w", err)
			}
		}
	}
}

func (c *HTTPAgentctlClient) RestoreAgent(ctx context.Context, endpoint string) error {
	return c.post(ctx, endpoint, "/agent/restore", struct{}{})
}

func (c *HTTPAgentctlClient) Queue(ctx context.Context, endpoint string, discard bool) error {
	path := "/agent/queue/resume"
	if discard {
		path = "/agent/queue/discard"
	}
	return c.post(ctx, endpoint, path, struct{}{})
}
