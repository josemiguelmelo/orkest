package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	stageintegration "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration"
	stageintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration/model"
)

type callbackIngestorFunc func(context.Context, string, stageintegrationmodel.CallbackRequest) (stageintegrationmodel.IngestionReceipt, error)

func (f callbackIngestorFunc) Ingest(ctx context.Context, id string, in stageintegrationmodel.CallbackRequest) (stageintegrationmodel.IngestionReceipt, error) {
	return f(ctx, id, in)
}

func TestWorkflowStageCallbackPreservesOpaqueBytesAndHeaders(t *testing.T) {
	for _, header := range []string{"X-Provider-One-Signature", "X-Provider-Two-Token"} {
		t.Run(header, func(t *testing.T) {
			body := []byte(" { \"action\": \"anything\" }\n")
			called := false
			api := workflowStageCallbackAPI{ingestor: callbackIngestorFunc(func(_ context.Context, id string, input stageintegrationmodel.CallbackRequest) (stageintegrationmodel.IngestionReceipt, error) {
				called = true
				if id != "connection" || input.Method != http.MethodPost || !bytes.Equal(body, input.Body) || http.Header(input.Headers).Get(header) != "opaque-secret" {
					t.Fatalf("callback input changed: id=%s method=%s", id, input.Method)
				}
				return stageintegrationmodel.IngestionReceipt{}, nil
			})}
			router := chi.NewRouter()
			router.Post("/callbacks/workflow-stage-integrations/{connectionID}", api.callback)
			request := httptest.NewRequest(http.MethodPost, "/callbacks/workflow-stage-integrations/connection", bytes.NewReader(body))
			request.Header.Set(header, "opaque-secret")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if !called || response.Code != http.StatusAccepted {
				t.Fatalf("called=%v status=%d", called, response.Code)
			}
		})
	}
}

func TestWorkflowStageCallbackResponses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		receipt stageintegrationmodel.IngestionReceipt
		err     error
		status  int
	}{
		{name: "duplicate", receipt: stageintegrationmodel.IngestionReceipt{Duplicate: true}, status: 200},
		{name: "ignored", receipt: stageintegrationmodel.IngestionReceipt{Ignored: true}, status: 200},
		{name: "signature", err: stageintegration.ErrUnauthenticated, status: 401},
		{name: "missing", err: modelerrors.ErrNotFound, status: 404},
		{name: "malformed", err: modelerrors.ErrInvalid, status: 400},
		{name: "storage", err: errors.New("sensitive storage detail"), status: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := workflowStageCallbackAPI{ingestor: callbackIngestorFunc(func(context.Context, string, stageintegrationmodel.CallbackRequest) (stageintegrationmodel.IngestionReceipt, error) {
				return tc.receipt, tc.err
			})}
			response := httptest.NewRecorder()
			api.callback(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
			if response.Code != tc.status || strings.Contains(response.Body.String(), "sensitive") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestWorkflowStageCallbackRejectsOversizedBodyBeforeIngestion(t *testing.T) {
	api := workflowStageCallbackAPI{ingestor: callbackIngestorFunc(func(context.Context, string, stageintegrationmodel.CallbackRequest) (stageintegrationmodel.IngestionReceipt, error) {
		t.Fatal("ingestion called")
		return stageintegrationmodel.IngestionReceipt{}, nil
	})}
	response := httptest.NewRecorder()
	api.callback(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", stageintegration.MaxCallbackBytes+1))))
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d", response.Code)
	}
}
