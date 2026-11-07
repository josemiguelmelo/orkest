package opencodetest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fixtureQuestionTransport struct{ authorization string }

func (t fixtureQuestionTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("Authorization", t.authorization)
	return http.DefaultTransport.RoundTrip(request)
}

// fixtureAskUser drives the same HTTP MCP boundary used by fallback runtimes.
// The parent fixture process alone owns stdout, which remains ACP-only.
func fixtureAskUser(commandKey string) error {
	raw, err := fixtureMCPConfig()
	if err != nil {
		return err
	}
	var servers []struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		Headers []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"headers"`
	}
	if err := json.Unmarshal(raw, &servers); err != nil {
		return err
	}
	for _, server := range servers {
		if server.Name != "orkest_questions" {
			continue
		}
		var authorization string
		for _, header := range server.Headers {
			if header.Name == "Authorization" {
				authorization = header.Value
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		client := mcp.NewClient(&mcp.Implementation{Name: "runtime fixture", Version: "1"}, nil)
		session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: fixtureQuestionTransport{authorization}}}, nil)
		if err != nil {
			return err
		}
		defer func() { _ = session.Close() }()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ask_user", Arguments: map[string]any{
			"commandKey": commandKey, "operationId": "manager", "message": "Package manager?",
			"schema": map[string]any{"type": "object", "properties": map[string]any{"manager": map[string]any{"type": "string", "enum": []string{"npm", "pnpm"}}}, "required": []string{"manager"}},
		}})
		if err != nil {
			return err
		}
		if result.IsError {
			return errors.New("fixture question tool failed")
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return err
		}
		var answer struct {
			Action  string            `json:"action"`
			Content map[string]string `json:"content"`
		}
		if json.Unmarshal(encoded, &answer) != nil || answer.Action != "accept" || answer.Content["manager"] != "pnpm" {
			return errors.New("fixture did not receive its answer")
		}
		return nil
	}
	return errors.New("fixture question server missing")
}
