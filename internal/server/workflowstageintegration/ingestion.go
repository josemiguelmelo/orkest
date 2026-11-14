package workflowstageintegration

import (
	"context"
	"encoding/json"
	"errors"

	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/secretbox"
	stageintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration/model"
)

const MaxCallbackBytes = 1 << 20

func (s *WorkflowStageIntegrationService) Ingest(ctx context.Context, id string, request stageintegrationmodel.CallbackRequest) (stageintegrationmodel.IngestionReceipt, error) {
	if len(request.Body) > MaxCallbackBytes {
		return stageintegrationmodel.IngestionReceipt{}, problem.ErrInvalid
	}
	stored, err := s.repo.CallbackConnection(ctx, id)
	if err != nil {
		return stageintegrationmodel.IngestionReceipt{}, err
	}
	c := stored.Connection
	if !c.Enabled {
		return stageintegrationmodel.IngestionReceipt{}, problem.ErrNotFound
	}
	p := s.providers[c.Provider]
	if p == nil {
		return stageintegrationmodel.IngestionReceipt{}, problem.ErrNotFound
	}
	if s.box == nil {
		return stageintegrationmodel.IngestionReceipt{}, secretbox.ErrKeyUnavailable
	}
	secret, err := s.box.Open([]byte(stored.EncryptedSecret))
	if err != nil {
		return stageintegrationmodel.IngestionReceipt{}, errors.New("integration callback credential unavailable")
	}
	result, err := p.HandleCallback(ctx, stageintegrationmodel.CallbackInput{Request: request, Config: c.Config, Secret: secret})
	if err != nil {
		return stageintegrationmodel.IngestionReceipt{}, err
	}
	if result.Ignored {
		return stageintegrationmodel.IngestionReceipt{Ignored: true}, nil
	}
	if result.DeliveryID == "" || len(result.DeliveryID) > 200 || len(result.Events) > 100 {
		return stageintegrationmodel.IngestionReceipt{}, problem.ErrInvalid
	}
	for _, event := range result.Events {
		if event.ID == "" || len(event.ID) > 200 || event.ActionID == "" || event.Subject.ID == "" || event.Subject.RepositoryID == "" || event.OccurredAt.IsZero() || len(event.Facts) > 16000 || !json.Valid(event.Facts) {
			return stageintegrationmodel.IngestionReceipt{}, problem.ErrInvalid
		}
	}
	duplicate, err := s.repo.RecordReceipt(ctx, c, result)
	return stageintegrationmodel.IngestionReceipt{Duplicate: duplicate}, err
}
