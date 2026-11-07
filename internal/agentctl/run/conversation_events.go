package run

import (
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
)

// ensureJournalUnderEventLock requires eventMu, but never a caller-held state lock. It opens
// storage outside mu, then replays committed state before admitting new work.
func (c *conversation) ensureJournalUnderEventLock() error {
	c.mu.RLock()
	journal := c.journal
	config := *c.config
	replay := c.replayExistingJournal
	usesTaskSession := c.state.usesTaskSession
	c.mu.RUnlock()
	if journal != nil {
		return nil
	}
	journal, history, err := loadJournal(c.stateDir, config, replay)
	if err != nil {
		return err
	}
	state := conversationState{usesTaskSession: usesTaskSession}
	for _, event := range history {
		if err := state.validate(event); err != nil {
			_ = journal.Close()
			return err
		}
		state.apply(event)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.journal = journal
	c.state = state
	c.pending = make(map[string]*pendingPermission)
	c.questions = make(map[string]*pendingQuestion)
	return nil
}

// commitEventUnderEventLock requires eventMu to serialize snapshot, persistence, application,
// and publication. It owns its mu acquisitions; callers never hold mu across I/O.
func (c *conversation) commitEventUnderEventLock(event interaction.Event) error {
	c.mu.RLock()
	config := c.config
	event.OrganizationID = config.OrganizationID
	event.WorkspaceID = config.WorkspaceID
	event.TaskID = config.TaskID
	event.RunID = config.RunID
	event.TaskSessionID = config.TaskSessionID
	event.SessionID = c.session.SessionID()
	journal := c.journal
	validationErr := c.state.validate(event)
	c.mu.RUnlock()
	if validationErr != nil {
		c.interrupt()
		return validationErr
	}

	committed, err := journal.Append(event)
	if err != nil {
		c.interrupt()
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.apply(committed)
	c.subscriptions.Publish(committed)
	return nil
}

// Subscribe snapshots replay and registers live delivery under eventMu, keeping
// storage reads outside mu and preventing gaps between replay and live events.
func (c *conversation) Subscribe(after uint64) ([]interaction.Event, <-chan interaction.Event, func(), error) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.RLock()
	closed, journal := c.closed, c.journal
	c.mu.RUnlock()
	if closed {
		return nil, nil, nil, interaction.ErrConversation
	}
	events, err := journal.Replay(after)
	if err != nil {
		return nil, nil, nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, nil, nil, interaction.ErrConversation
	}
	channel := c.subscriptions.Add()
	stop := func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.subscriptions.Remove(channel)
	}
	return events, channel, stop, nil
}
