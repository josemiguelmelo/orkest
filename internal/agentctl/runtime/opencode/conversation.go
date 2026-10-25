package opencode

import (
	"context"
	"errors"

	acp "github.com/coder/acp-go-sdk"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

func (p *process) Prompt(ctx context.Context, text string, observer agentctl.Observer) (string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wrapped := agentctl.Observer{
		Update: func(update agentctl.Update) error {
			err := observer.Update(update)
			if err != nil {
				cancel()
			}
			return err
		},
		Permission: func(requestCtx context.Context, permission agentctl.Permission) (string, error) {
			permissionCtx, stop := context.WithCancel(requestCtx)
			defer stop()
			detach := context.AfterFunc(ctx, stop)
			defer detach()
			option, err := observer.Permission(permissionCtx, permission)
			if err != nil {
				cancel()
			}
			return option, err
		},
	}
	p.client.mu.Lock()
	if p.client.observer != nil {
		p.client.mu.Unlock()
		return "", errors.New("prompt already running")
	}
	p.client.observer = &wrapped
	p.client.mu.Unlock()
	defer func() { p.client.mu.Lock(); p.client.observer = nil; p.client.mu.Unlock() }()
	result, err := p.conn.Prompt(ctx, acp.PromptRequest{SessionId: acp.SessionId(p.sessionID), Prompt: []acp.ContentBlock{acp.TextBlock(text)}})
	if err != nil {
		return "", errors.New("opencode ACP prompt failed")
	}
	return string(result.StopReason), nil
}

func (p *process) Cancel(ctx context.Context) error {
	return p.conn.Cancel(ctx, acp.CancelNotification{SessionId: acp.SessionId(p.sessionID)})
}
