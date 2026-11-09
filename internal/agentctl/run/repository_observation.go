package run

import (
	"context"
	"fmt"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	"github.com/josemiguelmelo/orkest/internal/agentctl/repositoryobserver"
	observation "github.com/josemiguelmelo/orkest/internal/contracts/repositoryobservation"
)

type repositoryObservationLoop struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func (loop *repositoryObservationLoop) Stop() {
	if loop != nil {
		loop.cancel()
		<-loop.done
	}
}

func (c *conversation) initializeRepositoryObservation() error {
	if c.repositoryObservationState != nil {
		return nil
	}
	history, err := c.journal.Replay(0)
	if err != nil {
		return fmt.Errorf("replay repository observations: %w", err)
	}
	c.mu.RLock()
	config := *c.config
	c.mu.RUnlock()
	store, err := loadRepositoryObservations(c.stateDir, config.OrganizationID, config.WorkspaceID, config.TaskID)
	if err != nil {
		return err
	}
	snapshots := store.history()
	var replayed []observation.Snapshot
	for _, event := range history {
		if event.RepositoryObservation != nil {
			replayed = append(replayed, *event.RepositoryObservation)
		}
	}
	if err := store.save(replayed...); err != nil {
		return fmt.Errorf("save recovered repository observations: %w", err)
	}
	c.repositoryObservationStore = store
	c.repositoryObservationState = repositoryobserver.New(append(snapshots, replayed...))
	return nil
}

func (c *conversation) startRepositoryObservation() *repositoryObservationLoop {
	if c.repositoryScanner == nil {
		return nil
	}
	if err := c.initializeRepositoryObservation(); err != nil {
		c.logger.Error("repository observation state unavailable", append(c.logAttrs(""), "reason", "state_unavailable")...)
		c.observationDiagnostic("observation_state_unavailable")
		c.interrupt()
		return nil
	}
	c.scanRepositories(c.ctx)
	ctx, cancel := context.WithCancel(c.ctx)
	loop := &repositoryObservationLoop{cancel: cancel, done: make(chan struct{})}
	go c.observeRepositories(ctx, loop.done)
	return loop
}

func (c *conversation) observeRepositories(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.scanRepositories(ctx)
		}
	}
}

func (c *conversation) scanRepositories(ctx context.Context) {
	if c.repositoryObservationState == nil {
		return
	}
	scanCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	snapshots, err := c.repositoryScanner.Scan(scanCtx)
	if err != nil {
		if ctx.Err() == nil {
			c.logger.Warn("repository observation incomplete", append(c.logAttrs(""), "reason", "scan_failed")...)
			if c.repositoryObservationDiagnostic != "scan_failed" {
				c.observationDiagnostic("scan_failed")
			}
			c.repositoryObservationDiagnostic = "scan_failed"
		}
		return
	}
	c.repositoryObservationDiagnostic = ""
	changes := repositoryobserver.Compare(c.repositoryObservationState, snapshots)
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	for _, snapshot := range changes {
		if err := c.commitEventUnderEventLock(interaction.Event{Type: interaction.EventRepositoryObserved, RepositoryObservation: &snapshot}); err != nil {
			return
		}
	}
	if err := c.repositoryObservationStore.save(changes...); err != nil {
		c.interrupt()
		return
	}
	for _, snapshot := range changes {
		c.repositoryObservationState.Record(snapshot)
	}
}

func (c *conversation) observationDiagnostic(diagnostic string) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	_ = c.commitEventUnderEventLock(interaction.Event{Type: interaction.EventRepositoryObserved, RepositoryObservation: &observation.Snapshot{Checkout: observation.Checkout{Path: "."}, Branches: []observation.Branch{}, Diagnostic: diagnostic}})
}
