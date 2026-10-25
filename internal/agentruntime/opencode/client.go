package opencode

import (
	"context"
	"errors"

	acp "github.com/coder/acp-go-sdk"
)

// launchClient advertises no filesystem or terminal capabilities. Prompting is not exposed.
type launchClient struct{}

func (launchClient) ReadTextFile(context.Context, acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, errors.New("client operation unavailable during launch")
}

func (launchClient) WriteTextFile(context.Context, acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, errors.New("client operation unavailable during launch")
}

func (launchClient) RequestPermission(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return acp.RequestPermissionResponse{}, errors.New("client operation unavailable during launch")
}

func (launchClient) CreateTerminal(context.Context, acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, errors.New("client operation unavailable during launch")
}

func (launchClient) KillTerminal(context.Context, acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, errors.New("client operation unavailable during launch")
}

func (launchClient) TerminalOutput(context.Context, acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, errors.New("client operation unavailable during launch")
}

func (launchClient) ReleaseTerminal(context.Context, acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, errors.New("client operation unavailable during launch")
}

func (launchClient) WaitForTerminalExit(context.Context, acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, errors.New("client operation unavailable during launch")
}
func (launchClient) SessionUpdate(context.Context, acp.SessionNotification) error { return nil }
