package run

import (
	"context"
	"sync"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	"github.com/josemiguelmelo/orkest/internal/agentctl/repositoryobserver"
	observation "github.com/josemiguelmelo/orkest/internal/contracts/repositoryobservation"
)

// observeRepositories brackets a prompt; its final scan completes before terminal events.
func (c *conversation) observeRepositories() func() {
	if c.repositoryScanner == nil {
		return func() {}
	}
	if c.repositoryObserver == nil {
		history, err := c.journal.Replay(0)
		if err != nil {
			c.interrupt()
			return func() {}
		}
		c.mu.RLock()
		config := *c.config
		c.mu.RUnlock()
		store, err := loadRepositoryObservations(c.stateDir, config.OrganizationID, config.WorkspaceID, config.TaskID)
		if err != nil {
			c.logger.Error("repository observation state unavailable", append(c.logAttrs(""), "reason", "state_invalid")...)
			c.observationDiagnostic("observation_state_unavailable")
			return func() {}
		}
		c.repositoryObservationStore = store
		snapshots := store.history()
		for _, event := range history {
			if event.RepositoryObservation != nil {
				snapshots = append(snapshots, *event.RepositoryObservation)
				if err := store.save(*event.RepositoryObservation); err != nil {
					c.interrupt()
					return func() {}
				}
			}
		}
		c.repositoryObserver = repositoryobserver.New(c.repositoryScanner, snapshots)
	}
	lastDiagnostic := ""
	scan := func() {
		ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		defer cancel()
		err := c.repositoryObserver.Scan(ctx, func(snapshot observation.Snapshot) error {
			c.eventMu.Lock()
			defer c.eventMu.Unlock()
			if err := c.commitEventUnderEventLock(interaction.Event{Type: interaction.EventRepositoryObserved, RepositoryObservation: &snapshot}); err != nil {
				return err
			}
			if err := c.repositoryObservationStore.save(snapshot); err != nil {
				c.interrupt()
				return err
			}
			return nil
		})
		if err != nil && c.ctx.Err() == nil {
			c.logger.Warn("repository observation incomplete", append(c.logAttrs(""), "reason", "scan_failed")...)
			if lastDiagnostic != "scan_failed" {
				c.observationDiagnostic("scan_failed")
			}
			lastDiagnostic = "scan_failed"
		} else if err == nil {
			lastDiagnostic = ""
		}
	}
	scan()
	ctx, cancel := context.WithCancel(c.ctx)
	var work sync.WaitGroup
	work.Go(func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				scan()
			}
		}
	})
	return func() { cancel(); work.Wait(); scan() }
}

func (c *conversation) observationDiagnostic(diagnostic string) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	_ = c.commitEventUnderEventLock(interaction.Event{Type: interaction.EventRepositoryObserved, RepositoryObservation: &observation.Snapshot{Checkout: observation.Checkout{Path: "."}, Branches: []observation.Branch{}, Diagnostic: diagnostic}})
}
