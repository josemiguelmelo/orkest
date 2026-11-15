package httpapi

import (
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
)

type editorTicket struct {
	user       string
	scope      taskeditor.Scope
	generation string
	expires    time.Time
}

// EditorGateway must be routed on a dedicated origin. No platform cookies or
// authorization headers are forwarded to code-server. Tokens are memory-only.
type EditorGateway struct {
	service                *taskeditor.TaskEditorService
	origin, platformOrigin string
	mu                     sync.Mutex
	grants                 map[string]editorTicket
	sessions               map[string]editorTicket
}

func parseEditorOrigin(value string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil {
		return nil, errors.New("invalid editor origin")
	}
	loopback := strings.EqualFold(u.Hostname(), "localhost") || strings.HasSuffix(strings.ToLower(u.Hostname()), ".localhost")
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path != "" || u.Opaque != "" || (u.Scheme != "https" && (u.Scheme != "http" || !loopback)) {
		return nil, errors.New("editor origins must be HTTPS origins (HTTP loopback allowed)")
	}
	u.Host = strings.ToLower(u.Host)
	if (u.Scheme == "https" && u.Port() == "443") || (u.Scheme == "http" && u.Port() == "80") {
		u.Host = u.Hostname()
		if strings.Contains(u.Host, ":") {
			u.Host = "[" + u.Host + "]"
		}
	}
	return u, nil
}

func NewEditorGateway(service *taskeditor.TaskEditorService, origin, platformOrigin string) (*EditorGateway, error) {
	editorURL, err := parseEditorOrigin(origin)
	if err != nil {
		return nil, err
	}
	platformURL, err := parseEditorOrigin(platformOrigin)
	if err != nil {
		return nil, err
	}
	if service == nil || editorURL.Host == platformURL.Host {
		return nil, errors.New("editor and platform must use distinct HTTP authorities")
	}
	return &EditorGateway{service: service, origin: editorURL.String(), platformOrigin: platformURL.String(), grants: make(map[string]editorTicket), sessions: make(map[string]editorTicket)}, nil
}

// MatchesHost shares the constructor's canonical authority with server routing.
func (g *EditorGateway) MatchesHost(host string) bool {
	authority := strings.TrimPrefix(strings.TrimPrefix(g.origin, "https://"), "http://")
	defaultPort := ":80"
	if strings.HasPrefix(g.origin, "https://") {
		defaultPort = ":443"
	}
	return strings.EqualFold(strings.TrimSuffix(host, defaultPort), authority)
}

func (g *EditorGateway) routes(r chi.Router) {
	const path = "/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}/editor"
	r.Get(path, g.inspect)
	r.Post(path, g.open)
	r.Delete(path, g.stop)
}

func editorScope(r *http.Request) taskeditor.Scope {
	org, workspace, task := scopeParams(r)
	return taskeditor.Scope{OrganizationID: org, WorkspaceID: workspace, TaskID: task}
}

func (g *EditorGateway) mutation(w http.ResponseWriter, r *http.Request) bool {
	// Non-browser clients may supply Bearer auth; cookie callers must prove origin.
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") && r.Header.Get("Origin") == "" {
		return true
	}
	if r.Header.Get("Origin") != g.platformOrigin {
		writeError(w, http.StatusForbidden, "invalid_origin")
		return false
	}
	return true
}

func editorResult(w http.ResponseWriter, value any, err error) {
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case err == nil:
		respond(w, value, nil, http.StatusOK)
	case errors.Is(err, taskeditor.ErrOwner):
		writeError(w, http.StatusConflict, "editor_owned_by_another_user")
	case errors.Is(err, taskeditor.ErrGeneration):
		writeError(w, http.StatusGone, "editor_generation_expired")
	default:
		// Preserve established authorization error mapping; sanitize backend failures.
		if isEditorAuthorizationError(err) {
			respond(w, nil, err, http.StatusOK)
		} else {
			writeError(w, http.StatusServiceUnavailable, "editor_unavailable")
		}
	}
}

func (g *EditorGateway) inspect(w http.ResponseWriter, r *http.Request) {
	status, err := g.service.Inspect(r.Context(), currentUserID(r), editorScope(r))
	editorResult(w, status, err)
}

func (g *EditorGateway) open(w http.ResponseWriter, r *http.Request) {
	if !g.mutation(w, r) {
		return
	}
	scope := editorScope(r)
	user := currentUserID(r)
	status, err := g.service.Open(r.Context(), user, scope)
	if err != nil {
		editorResult(w, nil, err)
		return
	}
	launchURL := ""
	if status.State == "ready" {
		token, tokenErr := randomState()
		if tokenErr != nil {
			editorResult(w, nil, tokenErr)
			return
		}
		g.mu.Lock()
		g.prune()
		g.grants[token] = editorTicket{user: user, scope: scope, generation: status.Generation, expires: time.Now().Add(time.Minute)}
		g.mu.Unlock()
		launchURL = g.origin + "/launch#" + token
	}
	editorResult(w, struct {
		taskeditor.Status
		LaunchURL string `json:"launchUrl,omitempty"`
	}{status, launchURL}, nil)
}

func (g *EditorGateway) stop(w http.ResponseWriter, r *http.Request) {
	if !g.mutation(w, r) {
		return
	}
	err := g.service.Stop(r.Context(), currentUserID(r), editorScope(r))
	editorResult(w, taskeditor.Status{State: "stopped"}, err)
}

func (g *EditorGateway) prune() {
	now := time.Now()
	for key, ticket := range g.grants {
		if now.After(ticket.expires) {
			delete(g.grants, key)
		}
	}
	for key, ticket := range g.sessions {
		if now.After(ticket.expires) {
			delete(g.sessions, key)
		}
	}
}

func (g *EditorGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !g.MatchesHost(r.Host) {
		http.NotFound(w, r)
		return
	}
	switch {
	case r.URL.Path == "/launch" && r.Method == http.MethodGet:
		g.launch(w, r)
	case r.URL.Path == "/exchange" && r.Method == http.MethodPost:
		g.exchange(w, r)
	default:
		g.proxy(w, r)
	}
}

func (g *EditorGateway) launch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; connect-src 'self'; frame-ancestors "+g.platformOrigin+"; base-uri 'none'")
	_, _ = io.WriteString(w, `<!doctype html><title>Open task editor</title><p id="status">Opening editor…</p><script>const grant=location.hash.slice(1);history.replaceState(null,'','/launch');fetch('/exchange',{method:'POST',headers:{'Content-Type':'text/plain'},body:grant}).then(async r=>{if(!r.ok)throw Error();location.replace((await r.json()).path)}).catch(()=>document.getElementById('status').textContent='Editor access expired. Open VS Code again from the task.');</script>`)
}

func (g *EditorGateway) exchange(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != g.origin {
		writeError(w, http.StatusForbidden, "invalid_origin")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	g.mu.Lock()
	ticket, found := g.grants[string(raw)]
	delete(g.grants, string(raw))
	g.prune()
	g.mu.Unlock()
	if !found || time.Now().After(ticket.expires) {
		writeError(w, http.StatusGone, "grant_expired")
		return
	}
	if _, _, err = g.service.Access(r.Context(), ticket.user, ticket.scope, ticket.generation); err != nil {
		writeError(w, http.StatusForbidden, "access_denied")
		return
	}
	id, err := randomState()
	if err != nil {
		editorResult(w, nil, err)
		return
	}
	secret, err := randomState()
	if err != nil {
		editorResult(w, nil, err)
		return
	}
	ticket.expires = time.Now().Add(15 * time.Minute)
	g.mu.Lock()
	g.sessions[id+":"+secret] = ticket
	g.mu.Unlock()
	path := "/s/" + id + "/"
	http.SetCookie(w, &http.Cookie{Name: "orkest_editor", Value: secret, Path: path, HttpOnly: true, Secure: strings.HasPrefix(g.origin, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: 900})
	respond(w, map[string]string{"path": path}, nil, http.StatusOK)
}

func (g *EditorGateway) proxy(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 3)
	if len(parts) != 3 || parts[0] != "s" {
		http.NotFound(w, r)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != g.origin {
		writeError(w, http.StatusForbidden, "invalid_origin")
		return
	}
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") && r.Header.Get("Origin") != g.origin {
		writeError(w, http.StatusForbidden, "invalid_origin")
		return
	}
	cookie, err := r.Cookie("orkest_editor")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "editor_session_required")
		return
	}
	g.mu.Lock()
	ticket, found := g.sessions[parts[1]+":"+cookie.Value]
	g.mu.Unlock()
	if !found || time.Now().After(ticket.expires) {
		writeError(w, http.StatusUnauthorized, "editor_session_expired")
		return
	}
	instance, lease, err := g.service.Access(r.Context(), ticket.user, ticket.scope, ticket.generation)
	if err != nil {
		writeError(w, http.StatusForbidden, "access_denied")
		return
	}
	upstream, err := url.Parse(instance.Endpoint)
	if err != nil || upstream.Host == "" || upstream.User != nil || upstream.RawQuery != "" || upstream.Fragment != "" || (upstream.Scheme != "http" && upstream.Scheme != "https") {
		writeError(w, http.StatusServiceUnavailable, "editor_unavailable")
		return
	}
	ctx, cancel := context.WithDeadline(r.Context(), ticket.expires)
	defer cancel()
	stop := context.AfterFunc(lease, cancel)
	defer stop()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DisableKeepAlives = true
	transport.DialContext = func(dialCtx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(dialCtx, network, address)
		if err == nil {
			context.AfterFunc(ctx, func() { _ = conn.Close() })
		}
		return conn, err
	}
	defer transport.CloseIdleConnections()
	prefix := "/s/" + parts[1]
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(upstream)
			pr.Out.URL.Path = strings.TrimPrefix(pr.In.URL.Path, prefix)
			pr.Out.URL.RawPath = ""
			pr.Out.Host = upstream.Host
			pr.Out.Header.Del("X-Orkest-Editor-Relay")
			if instance.GatewayToken != "" {
				pr.Out.Header.Set("X-Orkest-Editor-Relay", instance.GatewayToken)
			}
			if pr.In.Header.Get("Origin") != "" {
				pr.Out.Header.Set("Origin", upstream.Scheme+"://"+upstream.Host)
			}
			for _, header := range []string{"Authorization", "Cookie", "Proxy-Authorization", "Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"} {
				pr.Out.Header.Del(header)
			}
		},
		Transport: transport, ErrorLog: log.New(io.Discard, "", 0),
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, proxyErr error) {
			slog.WarnContext(r.Context(), "task editor proxy connection failed",
				"organization_id", ticket.scope.OrganizationID,
				"workspace_id", ticket.scope.WorkspaceID,
				"task_id", ticket.scope.TaskID,
				"upstream_host", upstream.Host,
				"error", proxyErr,
			)
			writeError(w, http.StatusBadGateway, "editor_connection_unavailable")
		},
		ModifyResponse: func(response *http.Response) error {
			response.Header.Del("Set-Cookie")
			response.Header.Del("Access-Control-Allow-Origin")
			response.Header.Del("X-Frame-Options")
			policies := response.Header.Values("Content-Security-Policy")
			if len(policies) > 0 {
				response.Header.Del("Content-Security-Policy")
				for _, policy := range policies {
					response.Header.Add("Content-Security-Policy", frameAncestorsPolicy(policy, g.platformOrigin))
				}
			}
			if location := response.Header.Get("Location"); strings.HasPrefix(location, "/") {
				response.Header.Set("Location", prefix+location)
			} else if location != "" {
				u, err := url.Parse(location)
				if err != nil || u.IsAbs() {
					response.Header.Del("Location")
				}
			}
			return nil
		},
	}
	proxy.ServeHTTP(w, r.WithContext(ctx))
}

func frameAncestorsPolicy(policy, platformOrigin string) string {
	directives := strings.Split(policy, ";")
	found := false
	for index, directive := range directives {
		fields := strings.Fields(directive)
		if len(fields) == 0 || !strings.EqualFold(fields[0], "frame-ancestors") {
			continue
		}
		directives[index] = " frame-ancestors 'self' " + platformOrigin
		found = true
	}
	if !found {
		directives = append(directives, " frame-ancestors 'self' "+platformOrigin)
	}
	return strings.Join(directives, ";")
}

func isEditorAuthorizationError(err error) bool {
	return errors.Is(err, problem.ErrForbidden) || errors.Is(err, problem.ErrNotFound) || errors.Is(err, problem.ErrInvalid)
}
