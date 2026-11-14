package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
)

type gatewayTasks struct{ denied atomic.Bool }

func (t *gatewayTasks) AuthorizeExecution(context.Context, string, string, string, string, bool) error {
	if t.denied.Load() {
		return problem.ErrForbidden
	}
	return nil
}

func (*gatewayTasks) Get(context.Context, string, string, string, string) (taskmodel.Task, error) {
	return taskmodel.Task{}, nil
}

type gatewayInspector struct{}

func (gatewayInspector) Inspect(context.Context, string, string, string, []taskeditor.Checkout) (taskeditor.Inspection, error) {
	return taskeditor.Inspection{Roots: []taskeditor.Root{{Path: "/workspace/repositories/github/repo"}}}, nil
}

type gatewayIdle struct{}

func (gatewayIdle) EditorIdle(context.Context, string, string, string) (bool, error) {
	return true, nil
}

type gatewayBackend struct {
	instance taskeditor.Instance
	found    bool
	endpoint string
}

func (b *gatewayBackend) StartEditor(_ context.Context, s taskeditor.Spec) (taskeditor.Instance, error) {
	s.GatewayToken = "fixture-editor-token"
	s.ID = "id"
	s.Endpoint = b.endpoint
	b.instance = s.Instance
	b.found = true
	return b.instance, nil
}

func (b *gatewayBackend) InspectEditor(context.Context, taskeditor.Scope) (taskeditor.Instance, bool, error) {
	return b.instance, b.found, nil
}

func (b *gatewayBackend) StopEditor(context.Context, taskeditor.Instance) error {
	b.found = false
	return nil
}

func (b *gatewayBackend) ListEditors(context.Context) ([]taskeditor.Instance, error) { return nil, nil }

func gatewayFixture(t *testing.T, upstream string) (*EditorGateway, *taskeditor.TaskEditorService, *gatewayTasks, string) {
	t.Helper()
	tasks := &gatewayTasks{}
	service, err := taskeditor.NewTaskEditorService(tasks, gatewayInspector{}, &gatewayBackend{endpoint: upstream}, gatewayIdle{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	scope := taskeditor.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}
	status, err := service.Open(context.Background(), "user", scope)
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := NewEditorGateway(service, "http://localhost:8089", "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	gateway.grants["grant"] = editorTicket{user: "user", scope: scope, generation: status.Generation, expires: time.Now().Add(time.Minute)}
	return gateway, service, tasks, status.Generation
}

func exchangeEditor(t *testing.T, g *EditorGateway) (string, *http.Cookie) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, g.origin+"/exchange", strings.NewReader("grant"))
	req.Header.Set("Origin", g.origin)
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("exchange: %d %s", rec.Code, rec.Body.String())
	}
	var body struct{ Path string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Path, rec.Result().Cookies()[0]
}

func TestEditorGatewayGrantAndCredentialIsolation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Forwarded-Host") != "" {
			t.Error("browser credentials reached code-server")
		}
		if r.Header.Get("X-Orkest-Editor-Relay") != "fixture-editor-token" {
			t.Error("gateway relay credential was not derived server-side")
		}
		w.Header().Set("Set-Cookie", "private=secret")
		_, _ = io.WriteString(w, r.URL.Path)
	}))
	defer upstream.Close()
	gateway, _, tasks, _ := gatewayFixture(t, upstream.URL)
	path, cookie := exchangeEditor(t, gateway)
	replay := httptest.NewRequest(http.MethodPost, gateway.origin+"/exchange", strings.NewReader("grant"))
	replay.Header.Set("Origin", gateway.origin)
	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, replay)
	if rec.Code != http.StatusGone {
		t.Fatalf("grant replay: %d", rec.Code)
	}
	request := httptest.NewRequest(http.MethodGet, gateway.origin+path+"static/file", nil)
	request.AddCookie(cookie)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: "platform-token"})
	request.Header.Set("X-Orkest-Editor-Relay", "forged-relay-token")
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("X-Forwarded-Host", "private-platform")
	rec = httptest.NewRecorder()
	gateway.ServeHTTP(rec, request)
	if rec.Code != http.StatusOK || rec.Body.String() != "/static/file" || rec.Header().Get("Set-Cookie") != "" {
		t.Fatalf("proxy: %d %s", rec.Code, rec.Body.String())
	}
	tasks.denied.Store(true)
	rec = httptest.NewRecorder()
	gateway.ServeHTTP(rec, request)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("revoked authority: %d", rec.Code)
	}
}

func TestEditorGatewayRejectsOriginAndOldGeneration(t *testing.T) {
	gateway, service, _, _ := gatewayFixture(t, "http://127.0.0.1:1")
	path, cookie := exchangeEditor(t, gateway)
	request := httptest.NewRequest(http.MethodGet, gateway.origin+path, nil)
	request.AddCookie(cookie)
	request.Header.Set("Origin", "https://attacker.invalid")
	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, request)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("origin: %d", rec.Code)
	}
	if err := service.Stop(context.Background(), "user", taskeditor.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}); err != nil {
		t.Fatal(err)
	}
	request.Header.Del("Origin")
	rec = httptest.NewRecorder()
	gateway.ServeHTTP(rec, request)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("stale session: %d", rec.Code)
	}
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		r := httptest.NewRequest(method, "http://localhost:8080/editor", nil)
		rec = httptest.NewRecorder()
		if gateway.mutation(rec, r) {
			t.Fatal("cookie mutation accepted without origin")
		}
	}
}

func TestEditorGatewayClosesWebSocketOnHandoff(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		for {
			kind, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			if err = conn.Write(r.Context(), kind, data); err != nil {
				return
			}
		}
	}))
	defer upstream.Close()
	gateway, service, _, _ := gatewayFixture(t, upstream.URL)
	path, cookie := exchangeEditor(t, gateway)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Host = "localhost:8089"; gateway.ServeHTTP(w, r) }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+path, &websocket.DialOptions{HTTPHeader: http.Header{"Cookie": {cookie.String()}, "Origin": {gateway.origin}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	if err = conn.Write(ctx, websocket.MessageText, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = conn.Read(ctx); err != nil {
		t.Fatal(err)
	}
	unlock, err := service.BeforeExecution(ctx, "org", "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	if _, _, err = conn.Read(ctx); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("websocket not revoked promptly: %v", err)
	}
}

func TestEditorGatewayRequiresDistinctRoutingAuthorities(t *testing.T) {
	gateway, service, _, _ := gatewayFixture(t, "http://127.0.0.1:1")
	for _, origins := range [][2]string{
		{"https://localhost:8089", "http://localhost:8089"},
		{"https://editor.example:443", "https://editor.example"},
		{"https://EDITOR.example", "https://editor.example"},
	} {
		if _, err := NewEditorGateway(service, origins[0], origins[1]); err == nil {
			t.Fatalf("ambiguous origin routing accepted: %v", origins)
		}
	}
	if _, err := NewEditorGateway(service, gateway.origin, gateway.platformOrigin); err != nil {
		t.Fatal(err)
	}
}

func TestEditorGatewayHostRoutingUsesCanonicalAuthority(t *testing.T) {
	_, service, _, _ := gatewayFixture(t, "http://127.0.0.1:1")
	gateway, err := NewEditorGateway(service, "https://EDITOR.example:443", "https://app.example")
	if err != nil {
		t.Fatal(err)
	}
	if !gateway.MatchesHost("editor.example") || !gateway.MatchesHost("EDITOR.example:443") || gateway.MatchesHost("editor.example.attacker.invalid") {
		t.Fatal("gateway authority routing differs from origin normalization")
	}
}
