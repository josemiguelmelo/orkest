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
func prepareQuestionTool(paths agentruntime.ProvisioningPaths, descriptor agentruntime.RuntimeDescriptor, restoring bool) (*questiontool.Server, string, error) {
	if !descriptor.UseQuestionTool {
		return nil, "", nil
	}
	path := filepath.Join(paths.RuntimeRoot, "question-tool.json")
	state := questionToolState{Version: 1, RuntimeVersion: descriptor.Version, Address: "127.0.0.1:0", Token: questiontool.NewToken()}
	if restoring {
		var manifest sessionManifest
		raw, err := os.ReadFile(filepath.Join(paths.RuntimeRoot, "session.json"))
		if err != nil || json.Unmarshal(raw, &manifest) != nil {
			return nil, "", errors.New("question tool session manifest unavailable")
		}
		if manifest.Version < 4 {
			return nil, "", nil
		}
		raw, err = os.ReadFile(path)
		if err != nil || json.Unmarshal(raw, &state) != nil || state.Version != 1 || state.RuntimeVersion != descriptor.Version {
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
	settings.MCPServers = append(settings.MCPServers, configmodel.MCPServer{ID: "orkest-questions", Name: "orkest_questions", URL: tool.URL(), Headers: map[string]string{"Authorization": "Bearer " + token}})
	settings.Agent.Instructions += questionInstructionsV1
	return nil
}

// This text is part of the version-1 persisted fallback configuration.
const questionInstructionsV1 = "\n\nWhen you need user input, call ask_user from the orkest_questions MCP server and wait for its result. Use the commandKey from the current Orkest message, a stable operationId for retries, and an object JSON Schema with primitive fields. Never request secrets. A declined or cancelled question is not an answer."
