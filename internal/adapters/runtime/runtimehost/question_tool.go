package runtimehost

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/questiontool"
	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	"github.com/josemiguelmelo/orkest/internal/platform/durablefile"
)

type questionToolState struct {
	Version        int    `json:"version"`
	RuntimeVersion string `json:"runtimeVersion"`
	Address        string `json:"address"`
	Token          string `json:"token"`
}

// prepareQuestionTool preserves the endpoint and instructions of saved sessions.
// Legacy manifests never receive an implicit MCP server or new instructions.
func prepareQuestionTool(paths agentruntime.ProvisioningPaths, runtimeVersion string, restoring bool, toolVersion int) (*questiontool.Server, string, error) {
	if toolVersion == 0 {
		return nil, "", nil
	}
	path := filepath.Join(paths.RuntimeRoot, "question-tool.json")
	state := questionToolState{Version: toolVersion, RuntimeVersion: runtimeVersion, Address: "127.0.0.1:0", Token: questiontool.NewToken()}
	if restoring {
		raw, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(raw, &state) != nil || state.Version != toolVersion || state.RuntimeVersion != runtimeVersion {
			return nil, "", errors.New("saved question tool configuration unavailable or incompatible")
		}
		host, port, err := net.SplitHostPort(state.Address)
		portNumber, portErr := strconv.Atoi(port)
		if err != nil || portErr != nil || host != "127.0.0.1" || portNumber < 1 || portNumber > 65535 {
			return nil, "", errors.New("saved question tool address is invalid")
		}
	}
	server, err := questiontool.Start(state.Address, state.Token)
	if err != nil {
		return nil, "", err
	}
	if !restoring {
		state.Address = server.Address()
		encoded, _ := json.Marshal(state)
		if err := durablefile.Write(path, encoded); err != nil {
			_ = server.Close()
			return nil, "", err
		}
	}
	return server, state.Token, nil
}

func addQuestionTool(settings *agentruntime.ResolvedRuntimeSettings, tool *questiontool.Server, token string) error {
	if tool == nil {
		return nil
	}
	for _, server := range settings.MCPServers {
		if server.ID == "orkest-questions" || server.Name == "orkest_questions" {
			return errors.New("MCP server conflicts with reserved Orkest question tool")
		}
	}
	settings.MCPServers = append(settings.MCPServers, configmodel.MCPServer{ID: "orkest-questions", Name: "orkest_questions", URL: tool.URL(), Headers: map[string]string{"Authorization": "Bearer " + token}, Transport: "http"})
	settings.Agent.Instructions += questionInstructionsV1
	return nil
}

// This text is part of the version-1 persisted fallback configuration.
const questionInstructionsV1 = "\n\nWhen you need user input, call ask_user from the orkest_questions MCP server and wait for its result. Use the commandKey from the current Orkest message, a stable operationId for retries, and an object JSON Schema with primitive fields. Never request secrets. A declined or cancelled question is not an answer."
