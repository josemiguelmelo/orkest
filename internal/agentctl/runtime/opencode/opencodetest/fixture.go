// Package opencodetest provides the deterministic ACP subprocess used by contract tests.
package opencodetest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Run(version string) {
	if fixtureVersion := os.Getenv("ORKEST_FIXTURE_VERSION"); fixtureVersion != "" {
		version = fixtureVersion
	}
	claudeFixture := os.Getenv("ORKEST_CLAUDE_ACP_FIXTURE") == "1"
	codexFixture := os.Getenv("ORKEST_CODEX_ACP_FIXTURE") == "1"
	optionsFixture := claudeFixture || codexFixture
	if os.Getenv("ORKEST_ACP_FIXTURE") == "1" || optionsFixture {
		if len(os.Args) > 1 && os.Args[1] == "--version" {
			if os.Getenv("ORKEST_ACP_WRONG_VERSION") == "1" {
				fmt.Println("SECRET")
				os.Exit(0)
			}
			if claudeFixture && os.Getenv("ORKEST_FIXTURE_VERSION") == "" {
				fmt.Println("0.99.0")
			} else if codexFixture {
				fmt.Println("@agentclientprotocol/codex-acp " + version)
			} else {
				fmt.Println(version)
			}
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
				if optionsFixture {
					if err := writeSessionOptions(request.Params); err != nil {
						os.Exit(14)
					}
				}
				result = map[string]any{"sessionId": "fixture-session"}
			case "session/load":
				var params struct {
					SessionID string `json:"sessionId"`
				}
				if json.Unmarshal(request.Params, &params) != nil || params.SessionID != "fixture-session" {
					os.Exit(9)
				}
				if optionsFixture {
					if err := writeSessionOptions(request.Params); err != nil {
						os.Exit(14)
					}
				}
				if data, err := os.ReadFile(filepath.Join(fixtureDataDir(claudeFixture), "last-prompt")); err == nil {
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
				if err := os.WriteFile(filepath.Join(fixtureDataDir(claudeFixture), "last-prompt"), []byte(params.Prompt[0].Text), 0o600); err != nil {
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
						if codexFixture {
							data = []byte(os.Getenv("CODEX_CONFIG"))
						} else if optionsFixture {
							data, err = fixtureSystemPrompt()
						} else {
							data, err = os.ReadFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode", "agents", "work.md"))
						}
					case "probe:skills":
						if codexFixture {
							data = []byte(os.Getenv("CODEX_CONFIG"))
						} else if optionsFixture {
							data, err = fixtureSystemPrompt()
						} else {
							data, err = os.ReadFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode", "skills", "conformance", "SKILL.md"))
						}
					case "probe:http-mcp":
						if optionsFixture {
							data, err = fixtureMCPConfig()
						} else {
							data = []byte(os.Getenv("OPENCODE_CONFIG_CONTENT"))
						}
					}
					if err != nil {
						os.Exit(12)
					}
					fixtureSend(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "fixture-session", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": string(data)}}}})
				}
				if strings.HasPrefix(params.Prompt[0].Text, "remember:") {
					if err := os.WriteFile(filepath.Join(fixtureDataDir(claudeFixture), "memory"), []byte(strings.TrimPrefix(params.Prompt[0].Text, "remember:")), 0o600); err != nil {
						os.Exit(10)
					}
				}
				if params.Prompt[0].Text == "recall" {
					data, err := os.ReadFile(filepath.Join(fixtureDataDir(claudeFixture), "memory"))
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

func fixtureDataDir(claude bool) string {
	if claude {
		if os.Getenv("ORKEST_CODEX_ACP_FIXTURE") == "1" {
			return os.Getenv("XDG_DATA_HOME")
		}
		return os.Getenv("CLAUDE_CONFIG_DIR")
	}
	return os.Getenv("XDG_DATA_HOME")
}

func writeSessionOptions(params json.RawMessage) error {
	return os.WriteFile(filepath.Join(fixtureDataDir(true), "session-options.json"), params, 0o600)
}

func fixtureSystemPrompt() ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(fixtureDataDir(true), "session-options.json"))
	if err != nil {
		return nil, err
	}
	var params struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	if err = json.Unmarshal(data, &params); err != nil {
		return nil, err
	}
	return params.Meta["systemPrompt"], nil
}

func fixtureMCPConfig() ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(fixtureDataDir(true), "session-options.json"))
	if err != nil {
		return nil, err
	}
	var params struct {
		MCPServers []map[string]json.RawMessage `json:"mcpServers"`
	}
	if err = json.Unmarshal(data, &params); err != nil {
		return nil, err
	}
	return json.Marshal(params.MCPServers)
}
