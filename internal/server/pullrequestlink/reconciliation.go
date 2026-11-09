package pullrequestlink

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
)

type DiscoveryRepository struct {
	ID                   string
	ProviderRepositoryID string
	IntegrationID        string
	URL                  string
	Candidates           []Candidate
}

type DiscoveryTarget struct {
	Since        time.Time
	Integration  trigger.Integration
	Repositories []DiscoveryRepository
}

type DiscoveryStore interface {
	ClaimDiscovery(context.Context) (DiscoveryTarget, bool, error)
	DiscoveryFinished(context.Context, string, string) error
}

type Discoverer interface {
	Discover(context.Context, string, json.RawMessage, time.Time, []DiscoveryRepository) ([]trigger.Event, error)
}

type EventIngestor interface {
	Ingest(context.Context, trigger.Integration, string, []trigger.Event) error
}

// Reconciler performs provider reads outside transactions. Stable event identities
// share webhook deduplication, and ingestion rechecks the configuration version.
type Reconciler struct {
	store     DiscoveryStore
	ingestor  EventIngestor
	providers map[string]Discoverer
}

func NewReconciler(store DiscoveryStore, ingestor EventIngestor, providers map[string]Discoverer) *Reconciler {
	return &Reconciler{store: store, ingestor: ingestor, providers: providers}
}

func (r *Reconciler) ReconcileNext(ctx context.Context) (bool, error) {
	target, found, err := r.store.ClaimDiscovery(ctx)
	if err != nil || !found {
		return found, err
	}
	provider := r.providers[target.Integration.Provider]
	if provider == nil {
		return true, r.store.DiscoveryFinished(ctx, target.Integration.ID, "provider_discovery_unavailable")
	}
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	repositories := make([]DiscoveryRepository, 0, len(target.Repositories))
	for _, repository := range target.Repositories {
		candidates := make([]Candidate, 0, len(repository.Candidates))
		for _, candidate := range repository.Candidates {
			if candidate.CanAssociate() {
				candidates = append(candidates, candidate)
			}
		}
		if len(candidates) > 0 {
			repository.Candidates = candidates
			repositories = append(repositories, repository)
		}
	}
	events, err := provider.Discover(requestCtx, target.Integration.WorkspaceID, target.Integration.Config, target.Since, repositories)
	diagnostic := ""
	if err != nil {
		diagnostic = "provider_discovery_failed"
	} else if err = r.ingestor.Ingest(ctx, target.Integration, "reconciliation", events); err != nil {
		diagnostic = "discovery_ingestion_failed"
	}
	if finishErr := r.store.DiscoveryFinished(ctx, target.Integration.ID, diagnostic); finishErr != nil {
		return true, finishErr
	}
	return true, err
}

func (r *Reconciler) Run(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		more, err := r.ReconcileNext(ctx)
		if err != nil {
			logger.Warn("pull request reconciliation incomplete", "reason", "provider_reconciliation_failed")
		}
		if more && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
