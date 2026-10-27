package runtimehost

import (
	"context"
	"errors"
	"sync"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

type sessionCallbacks struct {
	mu             sync.Mutex
	secrets        []string
	observer       *agentruntime.PromptObserver
	cancelObserver context.CancelFunc
	promptContext  context.Context
	replay         *historyCollector
	observerFailed bool
}

func (c *sessionCallbacks) Update(update interaction.Update) error {
	c.mu.Lock()
	observer := c.observer
	cancel := c.cancelObserver
	c.mu.Unlock()
	if observer == nil || observer.Update == nil {
		return nil
	}
	if update.Kind == "tool_call" || update.Kind == "tool_call_update" {
		update = interaction.BoundToolUpdate(interaction.RedactToolUpdate(update, c.secrets))
	}
	err := observer.Update(update)
	if err != nil && cancel != nil {
		c.mu.Lock()
		c.observerFailed = true
		c.mu.Unlock()
		cancel()
	}
	return err
}

func (c *sessionCallbacks) Permission(ctx context.Context, p interaction.Permission) (string, error) {
	c.mu.Lock()
	observer := c.observer
	cancel := c.cancelObserver
	promptContext := c.promptContext
	c.mu.Unlock()
	if observer == nil || observer.Permission == nil {
		return "", errors.New("no active approval handler")
	}
	permissionCtx, stop := context.WithCancel(ctx)
	detach := context.AfterFunc(promptContext, stop)
	defer func() { detach(); stop() }()
	selected, err := observer.Permission(permissionCtx, p)
	if err != nil && cancel != nil {
		c.mu.Lock()
		c.observerFailed = true
		c.mu.Unlock()
		cancel()
	}
	return selected, err
}

func (c *sessionCallbacks) Observation(observation agentruntime.HistoryObservation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.replay != nil {
		c.replay.Add(observation)
	}
}

func (c *sessionCallbacks) beginReplay(sessionID string, coverageVerified bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.replay = newHistoryCollector(sessionID, coverageVerified)
}

// finishReplay seals the evidence after LoadSession has delivered its replay.
// Later live observations cannot alter the restored checkpoint.
func (c *sessionCallbacks) finishReplay() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.replay != nil {
		c.replay.Add(agentruntime.HistoryObservation{Kind: agentruntime.HistoryReplayDone})
	}
}

func (c *sessionCallbacks) recoveryDecision(command interaction.Command) agentruntime.RecoveryDecision {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.replay.Decide(command)
}

func (c *sessionCallbacks) attachObserver(ctx context.Context, cancel context.CancelFunc, observer agentruntime.PromptObserver) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.observer = &observer
	c.cancelObserver = cancel
	c.promptContext = ctx
	c.observerFailed = false
}

func (c *sessionCallbacks) detachObserver() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.observer = nil
	c.cancelObserver = nil
	c.promptContext = nil
}

func (c *sessionCallbacks) hasObserverFailure() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.observerFailed
}
