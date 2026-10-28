// Package claudecodetest provides the deterministic ACP subprocess used by contract tests.
package claudecodetest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Run(version string) {
	if os.Getenv("ORKEST_ACP_FIXTURE") == "1" {
		if os.Args[1] == "--version" {
			if os.Getenv("ORKEST_ACP_WRONG_VERSION") == "1" {
				fmt.Println("SECRET")
				os.Exit(0)
			}
			fmt.Println(version)
			os.Exit(0)
		}
		var activePrompt any
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var request struct {
				ID     any             `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if json.Unmarshal(scanner.Bytes(), &request) != nil {
				os.Exit(2)
			}
			var result any
			switch request.Method {
			case "initialize":
				result = map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{"loadSession": os.Getenv("ORKEST_ACP_NO_RESTORE") != "1"}, "authMethods": []any{}}
			case "session/new":
				result = map[string]any{"sessionId": "fixture-session"}
			case "session/load":
				var params struct {
					SessionID string `json:"sessionId"`
				}
				if json.Unmarshal(request.Params, &params) != nil || params.SessionID != "fixture-session" {
					os.Exit(9)
				}
				if data, err := os.ReadFile(filepath.Join(os.Getenv("XDG_DATA_HOME"), "last-prompt")); err == nil {
					fixtureSend(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "fixture-session", "update": map[string]any{"sessionUpdate": "user_message_chunk", "content": map[string]any{"type": "text", "text": string(data)}}}})
				}
				fixtureSend(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "fixture-session", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "restored history"}}}})
				result = map[string]any{}
			case "session/set_mode":
				var params struct {
					ModeID string `json:"modeId"`
				}
				if json.Unmarshal(request.Params, &params) != nil || params.ModeID != "work" {
					os.Exit(8)
				}
				result = map[string]any{}
			case "session/prompt":
				var params struct {
					SessionID string `json:"sessionId"`
					Prompt    []struct {
						Text string `json:"text"`
					} `json:"prompt"`
				}
				if json.Unmarshal(request.Params, &params) != nil || params.SessionID != "fixture-session" || len(params.Prompt) != 1 {
					os.Exit(5)
				}
				if err := os.WriteFile(filepath.Join(os.Getenv("XDG_DATA_HOME"), "last-prompt"), []byte(params.Prompt[0].Text), 0o600); err != nil {
					os.Exit(13)
				}
				if _, text, ok := strings.Cut(params.Prompt[0].Text, "]\n\n"); ok {
					params.Prompt[0].Text = text
				}
				if strings.HasPrefix(params.Prompt[0].Text, "probe:") {
					var data []byte
					var err error
					switch params.Prompt[0].Text {
					case "probe:instructions":
						data, err = os.ReadFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "claude", "agents", "work.md"))
					case "probe:skills":
						data, err = os.ReadFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "claude", "skills", "conformance", "SKILL.md"))
					case "probe:http-mcp":
						data = []byte(os.Getenv("CLAUDE_CONFIG_CONTENT"))
					}
					if err != nil {
						os.Exit(12)
					}
					fixtureSend(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "fixture-session", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": string(data)}}}})
				}
				if strings.HasPrefix(params.Prompt[0].Text, "remember:") {
					if err := os.WriteFile(filepath.Join(os.Getenv("XDG_DATA_HOME"), "memory"), []byte(strings.TrimPrefix(params.Prompt[0].Text, "remember:")), 0o600); err != nil {
						os.Exit(10)
					}
				}
				if params.Prompt[0].Text == "recall" {
					data, err := os.ReadFile(filepath.Join(os.Getenv("XDG_DATA_HOME"), "memory"))
					if err != nil {
						os.Exit(11)
					}
					fixtureSend(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "fixture-session", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": string(data)}}}})
				}
				if params.Prompt[0].Text == "crash" {
					os.Exit(23)
				}
				fixtureSend(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "fixture-session", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "Hello from fixture"}}}})
				if params.Prompt[0].Text == "permission" {
					activePrompt = request.ID
					fixtureSend(map[string]any{"jsonrpc": "2.0", "id": "fixture-permission", "method": "session/request_permission", "params": map[string]any{"sessionId": "fixture-session", "toolCall": map[string]any{"toolCallId": "tool-1", "title": "Edit file"}, "options": []any{map[string]any{"optionId": "allow", "name": "Allow once", "kind": "allow_once"}, map[string]any{"optionId": "reject", "name": "Reject", "kind": "reject_once"}}}})
					continue
				}
				if params.Prompt[0].Text == "wait" {
					activePrompt = request.ID
					continue
				}
				result = map[string]any{"stopReason": "end_turn"}
			case "":
				if request.ID != "fixture-permission" {
					os.Exit(6)
				}
				fixtureSend(map[string]any{"jsonrpc": "2.0", "id": activePrompt, "result": map[string]any{"stopReason": "end_turn"}})
				activePrompt = nil
				continue
			case "session/cancel":
				fixtureSend(map[string]any{"jsonrpc": "2.0", "id": activePrompt, "result": map[string]any{"stopReason": "cancelled"}})
				activePrompt = nil
				continue
			default:
				os.Exit(3)
			}
			if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}); err != nil {
				os.Exit(4)
			}
		}
		os.Exit(0)
	}
}

func fixtureSend(value any) {
	if json.NewEncoder(os.Stdout).Encode(value) != nil {
		os.Exit(7)
	}
}
