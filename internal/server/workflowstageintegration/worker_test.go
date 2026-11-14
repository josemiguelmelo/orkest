package workflowstageintegration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	artifactmodel "github.com/josemiguelmelo/orkest/internal/server/artifact/model"
	codemodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
	stageintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type workerRepository struct {
	WorkflowStageIntegrationRepository
	candidate Candidate
	events    []stageintegrationmodel.ProviderEvent
	admitted  []stageintegrationmodel.ProviderEvent
}

func (r *workerRepository) Candidates(context.Context, time.Time) ([]Candidate, error) {
	return []Candidate{r.candidate}, nil
}

func (r *workerRepository) SaveObservation(_ context.Context, _ Candidate, o Observation) error {
	r.candidate.Observation = &o
	return nil
}

func (r *workerRepository) RecordEvents(_ context.Context, _ stageintegrationmodel.Connection, events []stageintegrationmodel.ProviderEvent) error {
	r.events = append(r.events, events...)
	return nil
}

func (r *workerRepository) Events(context.Context, Candidate, Observation) ([]stageintegrationmodel.ProviderEvent, error) {
	return r.events, nil
}

func (r *workerRepository) AdmitEvent(_ context.Context, _ Candidate, _ Observation, event stageintegrationmodel.ProviderEvent, _ string) error {
	r.admitted = append(r.admitted, event)
	return nil
}

type workerAccess struct {
	WorkspaceAccess
	err error
}

func (a *workerAccess) GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error) {
	return workspacemodel.Workspace{Role: "admin"}, a.err
}

type workerCoding struct{ CodingAccess }

func (workerCoding) CodingCredential(context.Context, string, string) (codemodel.Credentials, error) {
	return codemodel.Credentials{}, nil
}

type workerProvider struct {
	WorkflowStageIntegrationProvider
	subject     stageintegrationmodel.Subject
	events      []stageintegrationmodel.ProviderEvent
	err         error
	resolutions int
	reject      bool
	onResolve   func()
}

func (*workerProvider) Provider() string                                 { return "test" }
func (*workerProvider) ValidateParameters(string, json.RawMessage) error { return nil }
func (p *workerProvider) MatchSubjects(string, json.RawMessage, stageintegrationmodel.ProviderEvent, []stageintegrationmodel.Subject) (stageintegrationmodel.SubjectMatch, error) {
	return stageintegrationmodel.SubjectMatch{Matched: !p.reject}, nil
}

func (p *workerProvider) ResolveSubject(context.Context, json.RawMessage, string, string) (stageintegrationmodel.Subject, error) {
	p.resolutions++
	if p.onResolve != nil {
		p.onResolve()
	}
	return p.subject, p.err
}

func (p *workerProvider) Reconcile(context.Context, json.RawMessage, string, string, stageintegrationmodel.Subject, time.Time) (stageintegrationmodel.Reconciliation, error) {
	p.resolutions++
	return stageintegrationmodel.Reconciliation{Subject: p.subject, Events: p.events}, p.err
}

func workerFixture() (*WorkflowStageIntegrationService, *workerRepository, *workerProvider, *workerAccess) {
	p := &workerProvider{subject: stageintegrationmodel.Subject{Kind: "pull_request", ID: "pr", RepositoryID: "external-repo", URL: "https://example.test/pr", HeadSHA: "current-head"}}
	binding := workflowmodel.ArtifactInput{Name: "pr", SourceStageID: "build", OutputName: "pr", Required: true}
	repo := &workerRepository{candidate: Candidate{
		Connection: stageintegrationmodel.Connection{Provider: "test", CodingIntegrationID: "coding", RepositoryIDs: []string{"repo"}},
		StageID:    "review", Parameters: json.RawMessage(`{"artifactInputName":"pr"}`),
		Definition: workflowmodel.Definition{Stages: []workflowmodel.Stage{
			{ID: "build", ArtifactOutputs: []workflowmodel.ArtifactOutput{{Name: "pr", Kind: "pull_request"}}},
			{ID: "review", ArtifactInputs: []workflowmodel.ArtifactInput{binding}},
		}},
		AvailableArtifacts: []artifactmodel.Revision{{ID: "revision", Kind: "pull_request", StageID: "build", OutputName: "pr", StageVersion: 1, Content: artifactmodel.Content{URL: p.subject.URL}}},
		Repositories:       []codemodel.CodeRepository{{ID: "repo", IntegrationID: "coding", ProviderRepositoryID: "external-repo"}},
	}}
	access := &workerAccess{}
	return NewWorkflowStageIntegrationService(repo, nil, access, workerCoding{}, nil, p), repo, p, access
}

func TestProcessActivatesAfterAllInitialSubjectsAreVerified(t *testing.T) {
	for _, source := range []string{"webhook", "poll"} {
		t.Run(source, func(t *testing.T) {
			s, repo, provider, _ := workerFixture()
			started := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
			now := started
			s.now = func() time.Time { return now }
			provider.onResolve = func() { now = now.Add(5 * time.Second) }
			second := repo.candidate.AvailableArtifacts[0]
			second.ID = "second-revision"
			repo.candidate.AvailableArtifacts = append(repo.candidate.AvailableArtifacts, second)
			baselineEvents := []stageintegrationmodel.ProviderEvent{
				{ID: "during-first-read", Subject: provider.subject, OccurredAt: started.Add(time.Second)},
				{ID: "during-last-read", Subject: provider.subject, OccurredAt: started.Add(6 * time.Second)},
				{ID: "at-activation", Subject: provider.subject, OccurredAt: started.Add(10 * time.Second)},
			}
			if source == "webhook" {
				repo.events = baselineEvents
			} else {
				provider.events = baselineEvents
			}
			if err := s.Process(t.Context()); err != nil {
				t.Fatal(err)
			}
			activated := repo.candidate.Observation.ActivatedAt
			if !activated.Equal(now) || provider.resolutions != 2 {
				t.Fatalf("activation must follow all verification reads: activation=%s completed=%s reads=%d", activated, now, provider.resolutions)
			}
			if len(repo.admitted) != 0 {
				t.Fatal("event preceding completed baseline verification was admitted")
			}
			// Reconciliation must keep the baseline and still admit later activity.
			now = now.Add(31 * time.Second)
			later := stageintegrationmodel.ProviderEvent{ID: "after-activation", Subject: provider.subject, OccurredAt: activated.Add(time.Second)}
			if source == "webhook" {
				repo.events = append(repo.events, later)
			} else {
				provider.events = append(provider.events, later)
			}
			if err := s.Process(t.Context()); err != nil {
				t.Fatal(err)
			}
			if len(repo.admitted) != 1 || repo.admitted[0].ID != later.ID {
				t.Fatalf("expected only post-activation activity: %+v", repo.admitted)
			}
			if !repo.candidate.Observation.ActivatedAt.Equal(activated) {
				t.Fatal("reconciliation changed the activation baseline")
			}
		})
	}
}

func TestProcessAdmitsEventsAcrossSlowScans(t *testing.T) {
	for _, source := range []string{"webhook", "poll"} {
		t.Run(source, func(t *testing.T) {
			s, repo, provider, _ := workerFixture()
			now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
			s.now = func() time.Time { return now }
			if err := s.Process(t.Context()); err != nil {
				t.Fatal(err)
			}
			activated := repo.candidate.Observation.ActivatedAt
			for scan := 1; scan <= 3; scan++ {
				now = now.Add(31 * time.Second)
				event := stageintegrationmodel.ProviderEvent{ID: fmt.Sprint(scan), Subject: provider.subject, OccurredAt: now.Add(-time.Second)}
				event.Subject.HeadSHA = "stale-webhook-head"
				repo.events = nil
				provider.events = nil
				if source == "poll" {
					provider.events = []stageintegrationmodel.ProviderEvent{event}
				} else {
					repo.events = []stageintegrationmodel.ProviderEvent{event}
				}
				if err := s.Process(t.Context()); err != nil {
					t.Fatal(err)
				}
				if len(repo.admitted) != scan {
					t.Fatalf("scan %d: admitted %d events", scan, len(repo.admitted))
				}
				if repo.admitted[scan-1].Subject.HeadSHA != provider.subject.HeadSHA {
					t.Fatal("admission did not use verified head")
				}
				if !repo.candidate.Observation.ActivatedAt.Equal(activated) {
					t.Fatal("activation baseline changed")
				}
			}
			if provider.resolutions != 4 {
				t.Fatalf("expected one verification per scan, got %d", provider.resolutions)
			}
		})
	}
}

func TestProcessVerificationFailureBackoff(t *testing.T) {
	s, repo, provider, access := workerFixture()
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if err := s.Process(t.Context()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(31 * time.Second)
	repo.events = []stageintegrationmodel.ProviderEvent{{ID: "review", Subject: provider.subject, OccurredAt: now}}
	provider.err = errors.New("provider unavailable")
	if err := s.processCandidate(t.Context(), repo.candidate); err == nil {
		t.Fatal("verification failure hidden")
	}
	attempts := provider.resolutions
	now = now.Add(5 * time.Second)
	provider.err = nil
	if err := s.Process(t.Context()); err != nil {
		t.Fatal(err)
	}
	if provider.resolutions != attempts || len(repo.admitted) != 0 {
		t.Fatal("failure backoff bypassed")
	}
	now = now.Add(6 * time.Second)
	if err := s.Process(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(repo.admitted) != 1 {
		t.Fatal("event not admitted after recovery")
	}
	// A cached observation must still recheck authorization before event admission.
	now = now.Add(10 * time.Second)
	access.err = errors.New("access revoked")
	if err := s.processCandidate(t.Context(), repo.candidate); err == nil {
		t.Fatal("revoked access accepted")
	}
	if len(repo.admitted) != 1 {
		t.Fatal("admitted event without authorization")
	}
}

func TestProcessRejectsArtifactOfPreviousOutputKind(t *testing.T) {
	s, repo, provider, _ := workerFixture()
	repo.candidate.Definition.Stages[0].ArtifactOutputs[0].Kind = "code_review"
	if err := s.processCandidate(t.Context(), repo.candidate); err == nil {
		t.Fatal("old output kind accepted")
	}
	if provider.resolutions != 0 || len(repo.admitted) != 0 {
		t.Fatal("old artifact activated integration")
	}
	repo.candidate.AvailableArtifacts = append(repo.candidate.AvailableArtifacts, artifactmodel.Revision{
		ID: "new-revision", Kind: "code_review", StageID: "build", OutputName: "pr", StageVersion: 2, Content: artifactmodel.Content{URL: provider.subject.URL},
	})
	now := repo.candidate.Observation.AttemptedAt.Add(11 * time.Second)
	s.now = func() time.Time { return now }
	if err := s.processCandidate(t.Context(), repo.candidate); err != nil {
		t.Fatal(err)
	}
	if got := repo.candidate.Observation.RevisionIDs; len(got) != 1 || got[0] != "new-revision" {
		t.Fatalf("pinned revisions: %v", got)
	}
}

func TestUnrelatedEventsDoNotRefreshObservation(t *testing.T) {
	s, repo, provider, _ := workerFixture()
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if err := s.Process(t.Context()); err != nil {
		t.Fatal(err)
	}
	unrelated := provider.subject
	unrelated.ID = "another-pr"
	repo.events = []stageintegrationmodel.ProviderEvent{{ID: "other", Subject: unrelated, OccurredAt: now.Add(time.Second)}}
	now = now.Add(11 * time.Second)
	if err := s.Process(t.Context()); err != nil {
		t.Fatal(err)
	}
	if provider.resolutions != 1 || len(repo.admitted) != 0 {
		t.Fatalf("unrelated event caused work: calls=%d admissions=%d", provider.resolutions, len(repo.admitted))
	}
}

func TestUnmatchedEventsUseBoundedVerificationAndRemainEligible(t *testing.T) {
	s, repo, provider, _ := workerFixture()
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if err := s.Process(t.Context()); err != nil {
		t.Fatal(err)
	}
	provider.reject = true
	repo.events = []stageintegrationmodel.ProviderEvent{{ID: "review", Subject: provider.subject, OccurredAt: now.Add(time.Second)}}
	for range 30 {
		now = now.Add(time.Second)
		if err := s.Process(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if provider.resolutions != 4 || len(repo.admitted) != 0 {
		t.Fatalf("unbounded verification: calls=%d admissions=%d", provider.resolutions, len(repo.admitted))
	}
	provider.reject = false
	now = now.Add(10 * time.Second)
	if err := s.Process(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(repo.admitted) != 1 {
		t.Fatal("pending event lost while waiting for match")
	}
}
