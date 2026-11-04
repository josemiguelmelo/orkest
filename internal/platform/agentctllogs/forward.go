// Package agentctllogs forwards safe structured logs from agentctl processes.
package agentctllogs

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
)

// Forward reads structured agentctl records, dropping unstructured provider
// output and fields that could contain user content or credentials.
func Forward(reader io.Reader, attrs ...any) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	logger := slog.Default().With(append([]any{"component", "agentctl"}, attrs...)...)
	for scanner.Scan() {
		var record map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		var service, message, level string
		if json.Unmarshal(record["service"], &service) != nil || service != "agentctl" ||
			json.Unmarshal(record["msg"], &message) != nil || message == "" ||
			json.Unmarshal(record["level"], &level) != nil {
			continue
		}
		logAttrs := []any{"agentctl_message", message}
		for key, value := range record {
			if key == "service" || key == "msg" || key == "level" || key == "time" || sensitiveLogField(key) {
				continue
			}
			var decoded any
			if err := json.Unmarshal(value, &decoded); err == nil {
				logAttrs = append(logAttrs, "agentctl."+key, decoded)
			}
		}
		if timestamp, ok := record["time"]; ok {
			var value string
			if json.Unmarshal(timestamp, &value) == nil {
				logAttrs = append(logAttrs, "agentctl_time", value)
			}
		}
		logger.Log(context.Background(), agentctlLogLevel(level), "agentctl log", logAttrs...)
	}
	if err := scanner.Err(); err != nil {
		logger.Warn("agentctl log stream ended with a read error", "error", err)
	}
}

func agentctlLogLevel(value string) slog.Level {
	switch strings.ToUpper(value) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func sensitiveLogField(key string) bool {
	key = strings.ToLower(key)
	for _, fragment := range []string{"prompt", "content", "token", "secret", "password", "authorization", "header", "environment", "credential", "input", "output", "body"} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}
