// Package httplog provides structured HTTP request logging without recording
// request bodies, headers, or query strings.
package httplog

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

type contextKey struct{}

const traceIDHeader = "X-Trace-ID"

const slowRequestThreshold = 2 * time.Second

// LoggerFromContext returns the request-scoped logger, if one is present.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	logger, _ := ctx.Value(contextKey{}).(*slog.Logger)
	return logger
}

// TraceIDFromContext returns the request correlation ID propagated by
// Middleware.
func TraceIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(traceIDContextKey{}).(string)
	return id
}

// LoggerFromWriter returns the request-scoped logger associated with a
// response writer created by Middleware.
func LoggerFromWriter(w http.ResponseWriter) *slog.Logger {
	writer, _ := w.(*responseWriter)
	if writer == nil {
		return nil
	}
	return writer.logger
}

// Middleware logs every request with a generated correlation ID, method, path,
// status, and duration. It deliberately omits headers, query strings, and body.
func Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := requestTraceID(r)
			requestLogger := logger.With("trace_id", traceID, "method", r.Method, "path", r.URL.Path)
			w.Header().Set(traceIDHeader, traceID)
			start := time.Now()
			wrapped := &responseWriter{ResponseWriter: w, logger: requestLogger}
			ctx := context.WithValue(r.Context(), contextKey{}, requestLogger)
			ctx = context.WithValue(ctx, traceIDContextKey{}, traceID)
			r = r.WithContext(ctx)
			next.ServeHTTP(wrapped, r)
			status := wrapped.status
			if status == 0 {
				status = http.StatusOK
			}
			duration := time.Since(start)
			if status == http.StatusSwitchingProtocols {
				return
			}
			attrs := []any{"status", status, "duration_ms", duration.Milliseconds()}
			if status >= http.StatusInternalServerError {
				requestLogger.Error("http request completed", attrs...)
				return
			}
			if duration >= slowRequestThreshold {
				requestLogger.Warn("slow http request completed", attrs...)
			}
		})
	}
}

func requestTraceID(r *http.Request) string {
	provided := strings.ToLower(strings.TrimSpace(r.Header.Get(traceIDHeader)))
	if len(provided) == 32 {
		if _, err := hex.DecodeString(provided); err == nil {
			return provided
		}
	}
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(value[:])
}

type traceIDContextKey struct{}

// PropagatingHTTPClient forwards the active request trace ID on outgoing HTTP
// calls. It leaves the client's other settings and transport behavior intact.
func PropagatingHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	copy := *client
	transport := copy.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	copy.Transport = traceIDTransport{next: transport}
	return &copy
}

type traceIDTransport struct{ next http.RoundTripper }

func (t traceIDTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	traceID := TraceIDFromContext(r.Context())
	if traceID == "" {
		return t.next.RoundTrip(r)
	}
	request := r.Clone(r.Context())
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	request.Header.Set(traceIDHeader, traceID)
	return t.next.RoundTrip(request)
}

type responseWriter struct {
	http.ResponseWriter
	status int
	logger *slog.Logger
}

func (w *responseWriter) WriteHeader(status int) {
	if status >= 100 && status < 200 && status != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(value []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(value)
}

func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *responseWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	if w.status == 0 {
		w.status = http.StatusSwitchingProtocols
	}
	return hijacker.Hijack()
}
