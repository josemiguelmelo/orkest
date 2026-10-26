package orchestration

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

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

// HTTPAgentctlClient implements the agentctl REST and activity WebSocket APIs.
type HTTPAgentctlClient struct {
	http      *http.Client
	pollEvery time.Duration
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
				return fmt.Errorf("agentctl did not become healthy: %w", lastErr)
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *HTTPAgentctlClient) Status(ctx context.Context, endpoint string) (agentctl.Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/info", nil)
	if err != nil {
		return agentctl.Status{}, fmt.Errorf("create agentctl status request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return agentctl.Status{}, fmt.Errorf("read agentctl status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return agentctl.Status{}, fmt.Errorf("agentctl status endpoint returned HTTP %d", resp.StatusCode)
	}
	var status struct {
		Configured         bool           `json:"configured"`
		ExecutionAvailable bool           `json:"executionAvailable"`
		ProcessAlive       bool           `json:"processAlive"`
		State              agentctl.State `json:"state"`
		SessionID          string         `json:"sessionId"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&status); err != nil {
		return agentctl.Status{}, fmt.Errorf("decode agentctl status: %w", err)
	}
	return agentctl.Status{
		Configured: status.Configured, ExecutionAvailable: status.ExecutionAvailable,
		ProcessAlive: status.ProcessAlive, State: status.State, SessionID: status.SessionID,
	}, nil
}

func (c *HTTPAgentctlClient) Configure(ctx context.Context, endpoint string, config agentctl.RunConfig) error {
	return c.post(ctx, endpoint, "/agent/configure", config)
}

func (c *HTTPAgentctlClient) StartAgent(ctx context.Context, endpoint string) error {
	return c.post(ctx, endpoint, "/agent/launch", struct{}{})
}

func (c *HTTPAgentctlClient) Prompt(ctx context.Context, endpoint, commandID, prompt string) error {
	return c.post(ctx, endpoint, "/agent/prompt", struct {
		CommandID string `json:"commandId"`
		Text      string `json:"text"`
	}{CommandID: commandID, Text: prompt})
}

func (c *HTTPAgentctlClient) CancelPrompt(ctx context.Context, endpoint, commandID string) error {
	return c.post(ctx, endpoint, "/agent/cancel", struct {
		CommandID string `json:"commandId"`
	}{CommandID: commandID})
}

func (c *HTTPAgentctlClient) RespondPermission(ctx context.Context, endpoint string, response agentctl.PermissionResponse) error {
	return c.post(ctx, endpoint, "/agent/permissions/respond", response)
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
		return fmt.Errorf("agentctl request returned HTTP %d", resp.StatusCode)
	}
	return nil
}

type streamFrame struct {
	Type  string          `json:"type"`
	Event *agentctl.Event `json:"event,omitempty"`
}

func (c *HTTPAgentctlClient) Connect(ctx context.Context, endpoint string, cursor uint64, onConnected func(), onEvent func(agentctl.Event) error) error {
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
	connected := false
	for {
		var frame streamFrame
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
			if err = onEvent(*frame.Event); err != nil {
				return fmt.Errorf("persist agentctl event: %w", err)
			}
		}
	}
}
