package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/orchestration"
)

type tempAgentLaunchAPI struct{ launches *orchestration.LaunchService }

type tempAgentLaunchRequest struct {
	Config    agentctl.RunConfig `json:"config"`
	CommandID string             `json:"commandId"`
	Prompt    string             `json:"prompt"`
}

func (a tempAgentLaunchAPI) routes(r chi.Router) {
	r.Post("/temp/agent-launch", a.launch)
}

func (a tempAgentLaunchAPI) launch(w http.ResponseWriter, r *http.Request) {
	var request tempAgentLaunchRequest
	if !decodeBody(w, r, &request) {
		return
	}
	if err := request.Config.Validate(); err != nil ||
		strings.TrimSpace(request.CommandID) == "" || len(request.CommandID) > 200 ||
		strings.TrimSpace(request.Prompt) == "" || len([]byte(request.Prompt)) > 256*1024 {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	launch, err := a.launches.StartWithPrompt(r.Context(), request.Config, request.CommandID, request.Prompt)
	if err != nil {
		writeError(w, http.StatusBadGateway, "agent_launch_failed")
		return
	}
	respond(w, map[string]any{"launch": launch, "commandId": request.CommandID, "status": "accepted"}, nil, http.StatusAccepted)
}
