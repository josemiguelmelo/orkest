package acp

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func TestExplicitModelSelectionOnNewAndLoadedSessions(t *testing.T) {
	for _, test := range []struct {
		name      string
		load      bool
		reply     string
		wantError bool
	}{
		{name: "new", reply: "openrouter/openai/gpt-5.4-mini"},
		{name: "loaded", load: true, reply: "openrouter/openai/gpt-5.4-mini"},
		{name: "wrong model", reply: "opencode/pulse-alpha-free", wantError: true},
		{name: "loaded wrong model", load: true, reply: "opencode/pulse-alpha-free", wantError: true},
		{name: "selection rejected", reply: "reject", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			inputReader, inputWriter := io.Pipe()
			outputReader, outputWriter := io.Pipe()
			defer func() {
				_ = inputReader.Close()
				_ = inputWriter.Close()
				_ = outputReader.Close()
				_ = outputWriter.Close()
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			requests := make(chan map[string]any, 2)
			go func() {
				decoder := json.NewDecoder(inputReader)
				encoder := json.NewEncoder(outputWriter)
				for i := 0; i < 2; i++ {
					var request struct {
						ID     any            `json:"id"`
						Method string         `json:"method"`
						Params map[string]any `json:"params"`
					}
					if err := decoder.Decode(&request); err != nil {
						return
					}
					requests <- map[string]any{"method": request.Method, "params": request.Params}
					response := map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"sessionId": "session"}}
					if i == 1 {
						if test.reply == "reject" {
							delete(response, "result")
							response["error"] = map[string]any{"code": -32602, "message": "unavailable model"}
						} else {
							response["result"] = map[string]any{"configOptions": []any{map[string]any{"id": "model", "name": "Model", "type": "select", "currentValue": test.reply, "options": []any{}}}}
						}
					}
					if err := encoder.Encode(response); err != nil {
						return
					}
				}
			}()
			connection, err := (Factory{}).Connect(ctx, inputWriter, outputReader, &recordedCallbacks{})
			if err != nil {
				t.Fatal(err)
			}
			options := agentruntime.ProtocolSessionOptions{Model: "openrouter/openai/gpt-5.4-mini"}
			if test.load {
				err = connection.LoadSession(ctx, "session", "/work", options)
			} else {
				_, err = connection.NewSession(ctx, "/work", options)
			}
			if (err != nil) != test.wantError {
				t.Fatalf("selection error=%v, want error=%v", err, test.wantError)
			}
			first, second := <-requests, <-requests
			wantMethod := "session/new"
			if test.load {
				wantMethod = "session/load"
			}
			if first["method"] != wantMethod || second["method"] != "session/set_config_option" {
				t.Fatalf("requests: %+v %+v", first, second)
			}
			params := second["params"].(map[string]any)
			if params["configId"] != "model" || params["value"] != options.Model || params["sessionId"] != "session" {
				t.Fatalf("model request: %+v", params)
			}
		})
	}
}
