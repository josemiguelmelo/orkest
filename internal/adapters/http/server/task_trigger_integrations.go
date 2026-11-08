package httpapi

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
)

type taskTriggerIntegrationAPI struct {
	service *trigger.TaskTriggerIntegrationService
}

func (a taskTriggerIntegrationAPI) routes(r chi.Router) {
	r.Get("/task-trigger-integration-providers", func(w http.ResponseWriter, r *http.Request) { respond(w, a.service.Providers(), nil, http.StatusOK) })
	r.Route("/organizations/{orgID}/workspaces/{workspaceID}/task-trigger-integrations", func(r chi.Router) {
		r.Get("/", a.list)
		r.Post("/", a.save)
		r.Route("/{integrationID}", func(r chi.Router) {
			r.Put("/", a.save)
			r.Delete("/", a.delete)
			r.Get("/deliveries", a.deliveries)
			r.Get("/links", a.links)
			r.Post("/links", a.link)
			r.Delete("/links", a.unlink)
		})
	})
}

func triggerScope(r *http.Request) (string, string, string, string) {
	return currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "integrationID")
}

func (a taskTriggerIntegrationAPI) list(w http.ResponseWriter, r *http.Request) {
	user, org, workspace, _ := triggerScope(r)
	v, err := a.service.List(r.Context(), user, org, workspace)
	respond(w, v, err, http.StatusOK)
}

func (a taskTriggerIntegrationAPI) save(w http.ResponseWriter, r *http.Request) {
	var body trigger.Input
	if !decodeBody(w, r, &body) {
		return
	}
	user, org, workspace, id := triggerScope(r)
	v, err := a.service.Save(r.Context(), user, org, workspace, id, body)
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	respond(w, v, err, status)
}

func (a taskTriggerIntegrationAPI) delete(w http.ResponseWriter, r *http.Request) {
	user, org, workspace, id := triggerScope(r)
	respond(w, nil, a.service.Delete(r.Context(), user, org, workspace, id), http.StatusNoContent)
}

func (a taskTriggerIntegrationAPI) links(w http.ResponseWriter, r *http.Request) {
	user, org, workspace, id := triggerScope(r)
	v, err := a.service.Links(r.Context(), user, org, workspace, id)
	respond(w, v, err, http.StatusOK)
}

func (a taskTriggerIntegrationAPI) link(w http.ResponseWriter, r *http.Request) {
	var body trigger.ResourceLink
	if !decodeBody(w, r, &body) {
		return
	}
	user, org, workspace, id := triggerScope(r)
	respond(w, nil, a.service.Link(r.Context(), user, org, workspace, id, body), http.StatusNoContent)
}

func (a taskTriggerIntegrationAPI) unlink(w http.ResponseWriter, r *http.Request) {
	var body trigger.ResourceLink
	if !decodeBody(w, r, &body) {
		return
	}
	user, org, workspace, id := triggerScope(r)
	respond(w, nil, a.service.Unlink(r.Context(), user, org, workspace, id, body), http.StatusNoContent)
}

func (a taskTriggerIntegrationAPI) deliveries(w http.ResponseWriter, r *http.Request) {
	user, org, workspace, id := triggerScope(r)
	v, err := a.service.Deliveries(r.Context(), user, org, workspace, id)
	respond(w, v, err, http.StatusOK)
}

func (a taskTriggerIntegrationAPI) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		http.Error(w, "webhook body too large", http.StatusRequestEntityTooLarge)
		return
	}
	headers := make(map[string]string, len(r.Header))
	for key := range r.Header {
		headers[key] = r.Header.Get(key)
	}
	err = a.service.Receive(r.Context(), chi.URLParam(r, "integrationID"), trigger.WebhookRequest{Body: body, Headers: headers})
	respond(w, nil, err, http.StatusAccepted)
}
