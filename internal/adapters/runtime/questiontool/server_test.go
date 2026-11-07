package questiontool

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type authenticatedTransport struct{ token string }

func (t authenticatedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(r)
}

func TestMCPQuestionsAuthenticateDeduplicateAndRejectExpiredCommands(t *testing.T) {
	token := NewToken()
	server, err := Start("127.0.0.1:0", token)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { server.End(); _ = server.Close() }()
	response, err := http.Get(server.URL())
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("unauthenticated question tool exposed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	key := server.Begin(ctx, func(_ context.Context, q interaction.Question) (interaction.QuestionAnswer, error) {
		calls.Add(1)
		if q.Message != "Choose" {
			t.Error("message lost")
		}
		return interaction.QuestionAnswer{Action: "accept", Content: json.RawMessage(`{"manager":"pnpm"}`)}, nil
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL(), HTTPClient: &http.Client{Transport: authenticatedTransport{token}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	arguments := map[string]any{"commandKey": key, "operationId": "question", "message": "Choose", "schema": map[string]any{"type": "object", "properties": map[string]any{"manager": map[string]any{"type": "string"}}}}
	for range 2 {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ask_user", Arguments: arguments})
		if err != nil || result.IsError {
			t.Fatalf("MCP question failed: %+v %v", result, err)
		}
		encoded, _ := json.Marshal(result.StructuredContent)
		if !strings.Contains(string(encoded), `"manager":"pnpm"`) {
			t.Fatalf("answer lost: %s", encoded)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("identical retry created another question")
	}
	arguments["message"] = "Different"
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ask_user", Arguments: arguments})
	if err != nil || !result.IsError {
		t.Fatal("conflicting retry accepted", err)
	}
	server.End()
	server.Begin(ctx, func(context.Context, interaction.Question) (interaction.QuestionAnswer, error) {
		t.Error("old command reached new observer")
		return interaction.QuestionAnswer{}, nil
	})
	arguments["operationId"] = "late"
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "ask_user", Arguments: arguments})
	if err != nil || !result.IsError {
		t.Fatal("expired command accepted", err)
	}
}

func TestDisconnectDoesNotCancelPendingQuestion(t *testing.T) {
	s := &Server{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	key := s.Begin(ctx, func(ctx context.Context, _ interaction.Question) (interaction.QuestionAnswer, error) {
		close(started)
		<-ctx.Done()
		return interaction.QuestionAnswer{}, ctx.Err()
	})
	op, err := s.admit(askInput{CommandKey: key, OperationID: "operation", Message: "Question", Schema: map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}}})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	select {
	case <-op.done:
		t.Fatal("question ended without command cancellation")
	default:
	}
	s.End()
	select {
	case <-op.done:
	case <-time.After(time.Second):
		t.Fatal("command cancellation left a question waiting")
	}
}
