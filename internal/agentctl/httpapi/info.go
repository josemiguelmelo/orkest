package httpapi

import (
	"net/http"
	"runtime"
	"time"
)

type Info struct {
	Service            string    `json:"service"`
	APIVersion         string    `json:"apiVersion"`
	GoVersion          string    `json:"goVersion"`
	OS                 string    `json:"os"`
	Arch               string    `json:"arch"`
	StartedAt          time.Time `json:"startedAt"`
	Configured         bool      `json:"configured"`
	ExecutionAvailable bool      `json:"executionAvailable"`
}

func (s *Server) snapshot() Info {
	status := s.service.Status()
	return Info{Service: "agentctl", APIVersion: "v1", GoVersion: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH, StartedAt: s.startedAt, Configured: status.Configured, ExecutionAvailable: status.ExecutionAvailable}
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.snapshot()) }
