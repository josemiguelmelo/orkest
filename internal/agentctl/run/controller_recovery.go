package run

import (
	runmodel "github.com/josemiguelmelo/orkest/internal/agentctl/run/model"
)

// CanAutoRestore keeps recovery policy with the controller; the daemon
// only schedules periodic supervision.
func (c *RunController) CanAutoRestore() bool {
	status := c.Status()
	c.mu.RLock()
	credentialsReady := c.runConfig != nil && (!c.runConfig.GitCredentialsRequired || len(c.runConfig.GitCredentials) > 0)
	c.mu.RUnlock()
	return credentialsReady && !status.RecoveryBlocked && status.RecoveryPermitted && !status.RunCancelled && !status.RunEnded && status.TaskSessionID != "" && status.SessionID != "" &&
		!status.ProcessAlive && (status.State == runmodel.StateExited || status.State == runmodel.StateFailed || status.State == runmodel.StateInterrupted)
}

// RestoreSession serializes replacement of a terminated runtime session.
func (c *RunController) RestoreSession() error {
	if !c.restoreMu.TryLock() {
		return ErrLaunchConflict
	}
	defer c.restoreMu.Unlock()
	c.mu.Lock()
	if c.runConfig != nil && c.runConfig.GitCredentialsRequired && len(c.runConfig.GitCredentials) == 0 {
		c.mu.Unlock()
		return ErrNotConfigured
	}
	if c.checkpointBlocked {
		c.mu.Unlock()
		return ErrRecoveryBlocked
	}
	if c.closed || c.launchState == runmodel.StateStarting {
		c.mu.Unlock()
		return ErrLaunchConflict
	}
	if c.conversation.Snapshot().RunEnded {
		c.mu.Unlock()
		return ErrLaunchConflict
	}
	var oldConversation *conversation
	if c.launchAttempted && c.session != nil {
		select {
		case <-c.session.Done():
			oldConversation = c.conversation
			c.conversation = newConversation(c.ctx, c.stateDir, c.logger)
			c.session = nil
		default:
			c.mu.Unlock()
			return ErrLaunchConflict
		}
	}
	if c.launchAttempted && c.session == nil {
		c.launchAttempted = false
	}
	if c.runConfig == nil || c.host == nil {
		c.mu.Unlock()
		return ErrNotConfigured
	}
	c.recoveryBlocked = false
	c.launchAttempted, c.launchState = true, runmodel.StateStarting
	config := *c.runConfig
	c.startupWorkers.Add(1)
	c.mu.Unlock()
	defer c.startupWorkers.Done()
	if oldConversation != nil {
		_ = oldConversation.close()
	}
	return c.startReservedSession(restoreSession, config)
}

// RecoverFromCheckpoint restores the last bound run on daemon restart. Runtime ownership must
// be acquired before any prompt is admitted. Finished runs are never resumed.
func (c *RunController) RecoverFromCheckpoint(expectedRunID string) (result error) {
	c.checkpointMu.Lock()
	defer c.checkpointMu.Unlock()
	c.mu.Lock()
	if c.closed || c.launchAttempted {
		c.mu.Unlock()
		return ErrLaunchConflict
	}
	c.checkpointBlocked = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if result != nil && c.checkpointBlocked {
			c.recoveryBlocked, c.launchState = true, runmodel.StateInterrupted
		}
	}()
	config, found, err := c.checkpoints.Load(expectedRunID)
	if err != nil {
		return err
	}
	if !found {
		c.mu.Lock()
		c.checkpointBlocked, c.recoveryBlocked = false, false
		c.mu.Unlock()
		return nil
	}
	if err := c.configure(config, true); err != nil {
		return err
	}
	if config.GitCredentialsRequired && len(config.GitCredentials) == 0 {
		c.mu.Lock()
		c.checkpointBlocked, c.recoveryBlocked = false, false
		c.mu.Unlock()
		return nil
	}
	if config.TaskSessionID != "" {
		ended, err := journalRunEnded(c.stateDir, config)
		if err != nil {
			return err
		}
		if ended {
			c.mu.Lock()
			c.checkpointBlocked, c.recoveryBlocked = false, false
			c.launchAttempted, c.launchState = true, runmodel.StateExited
			c.conversation.markRecoveredRunEnded()
			c.mu.Unlock()
			return nil
		}
	}
	c.mu.Lock()
	c.checkpointBlocked, c.recoveryBlocked = false, false
	c.mu.Unlock()
	return c.RestoreSession()
}

// CheckpointRecoveryPending permits retrying the saved checkpoint only. It never
// authorizes creating a session when the checkpoint cannot be read or validated.
func (c *RunController) CheckpointRecoveryPending() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.checkpointBlocked && !c.closed
}
