// Package questiontool exposes user questions to runtimes through a private HTTP MCP tool.
package questiontool

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	"github.com/josemiguelmelo/orkest/internal/platform/form"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Ask func(context.Context, interaction.Question) (interaction.QuestionAnswer, error)

type operation struct {
	input  string
	done   chan struct{}
	answer interaction.QuestionAnswer
	err    error
}

type Server struct {
	mu         sync.Mutex
	work       sync.WaitGroup
	closeOnce  sync.Once
	closeErr   error
	closed     bool
	server     *http.Server
	address    string
	token      string
	commandKey string
	ctx        context.Context
	cancel     context.CancelFunc
	ask        Ask
	operations map[string]*operation
}

func NewToken() string {
	value := make([]byte, 32)
	_, _ = rand.Read(value)
	return base64.RawURLEncoding.EncodeToString(value)
}

// Start binds only loopback. A saved nonzero address must be rebound exactly on restore.
func Start(address, token string) (*Server, error) {
	if len(token) < 32 {
		return nil, errors.New("question tool credential is required")
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		return nil, errors.New("question tool must use loopback")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, errors.New("question tool address is unavailable; saved session preserved")
	}
	s := &Server{address: listener.Addr().String(), token: token}
	protocol := mcp.NewServer(&mcp.Implementation{Name: "Orkest questions", Version: "1.0.0"}, nil)
	mcp.AddTool(protocol, &mcp.Tool{Name: "ask_user", Description: "Ask the user for information and wait for their answer. Use the commandKey supplied with the current Orkest message. Reuse operationId only for an identical retry. Never guess an answer or request secrets.", OutputSchema: answerSchema()}, s.handle)
	stream := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return protocol }, &mcp.StreamableHTTPOptions{Stateless: true})
	s.server = &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
		stream.ServeHTTP(w, r)
	})}
	go func() { _ = s.server.Serve(listener) }()
	return s, nil
}

func answerSchema() map[string]any {
	return map[string]any{
		"type": "object", "required": []string{"action"},
		"properties": map[string]any{
			"action":  map[string]any{"type": "string", "enum": []string{"accept", "decline", "cancel"}},
			"content": map[string]any{"type": "object", "additionalProperties": true},
		},
	}
}

func (s *Server) URL() string     { return "http://" + s.address + "/mcp" }
func (s *Server) Address() string { return s.address }
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		s.End()
		s.closeErr = s.server.Close()
		s.work.Wait()
	})
	return s.closeErr
}

// Begin binds tool calls to this exact turn; expired keys cannot target a later turn.
func (s *Server) Begin(ctx context.Context, ask Ask) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ""
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.commandKey, s.ask = NewToken(), ask
	s.operations = make(map[string]*operation)
	return s.commandKey
}

func (s *Server) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.commandKey, s.ask, s.ctx = "", nil, nil
}

type askInput struct {
	CommandKey  string         `json:"commandKey"`
	OperationID string         `json:"operationId"`
	Message     string         `json:"message"`
	Schema      map[string]any `json:"schema"`
}

func (s *Server) handle(ctx context.Context, _ *mcp.CallToolRequest, input askInput) (*mcp.CallToolResult, interaction.QuestionAnswer, error) {
	op, err := s.admit(input)
	if err != nil {
		return nil, interaction.QuestionAnswer{}, err
	}
	select {
	case <-op.done:
		return nil, op.answer, op.err
	case <-ctx.Done():
		// Losing the HTTP request does not cancel the worker-owned question.
		return nil, interaction.QuestionAnswer{}, ctx.Err()
	}
}

func (s *Server) admit(input askInput) (*operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.commandKey == "" || subtle.ConstantTimeCompare([]byte(input.CommandKey), []byte(s.commandKey)) != 1 || s.ctx.Err() != nil || input.OperationID == "" || len(input.OperationID) > 200 {
		return nil, errors.New("question is outside the active command")
	}
	raw, err := json.Marshal(input.Schema)
	if err != nil || len(raw) > form.MaxBytes || len(input.Message) > form.MaxBytes {
		return nil, errors.New("invalid question")
	}
	fingerprint, _ := json.Marshal(input)
	if previous := s.operations[input.OperationID]; previous != nil {
		if previous.input != string(fingerprint) {
			return nil, errors.New("question operationId conflicts with an earlier request")
		}
		return previous, nil
	}
	if len(s.operations) >= 100 {
		return nil, errors.New("question limit reached for this command")
	}
	op := &operation{input: string(fingerprint), done: make(chan struct{})}
	s.operations[input.OperationID] = op
	ask, commandContext := s.ask, s.ctx
	s.work.Add(1)
	go func() {
		defer s.work.Done()
		op.answer, op.err = ask(commandContext, interaction.Question{Message: input.Message, Schema: raw})
		close(op.done)
	}()
	return op, nil
}
