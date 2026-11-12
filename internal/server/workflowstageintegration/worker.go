package workflowstageintegration

import (
	"context"
	"errors"
	"time"

	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
)

// Process scans current observations. Each external call happens before a short,
// version-checked persistence transaction. One bad connection never starves peers.
func (s *WorkflowStageIntegrationService) Process(ctx context.Context) error {
	candidates, err := s.repo.Candidates(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.processCandidate(ctx, candidate); err != nil {
			if s.logger != nil {
				s.logger.Warn("workflow stage observation requires attention", "connection_id", candidate.Connection.ID, "task_id", candidate.TaskID, "reason", "observation_failed")
			}
		}
	}
	return nil
}

func (s *WorkflowStageIntegrationService) processCandidate(ctx context.Context, c Candidate) error {
	p, ok := s.providers[c.Connection.Provider]
	if !ok {
		return problem.ErrInvalid
	}
	resolver, ok := p.(SubjectProvider)
	if !ok {
		return problem.ErrInvalid
	}
	o := Observation{ID: c.ObservationID, TaskID: c.TaskID, RuleID: c.RuleID, StageVersion: c.StageVersion, Subjects: []Subject{}, RevisionIDs: []string{}}
	if c.Observation != nil {
		o = *c.Observation
		o.Subjects = append([]Subject(nil), o.Subjects...)
		o.RevisionIDs = append([]string(nil), o.RevisionIDs...)
	}
	// Rejected observations retry bounded read-only verification, not execution.
	due := (o.ReconciledAt.IsZero() || time.Since(o.ReconciledAt) >= 30*time.Second) && (o.AttemptedAt.IsZero() || time.Since(o.AttemptedAt) >= 10*time.Second)
	if due {
		now := time.Now().UTC()
		initial := len(o.Subjects) == 0
		err := s.refreshObservation(ctx, p, resolver, c, &o, now)
		if err != nil && initial {
			o.Subjects = []Subject{}
			o.RevisionIDs = []string{}
			o.ActivatedAt = time.Time{}
		}
		o.AttemptedAt = now
		if err == nil {
			o.ReconciledAt = now
		}
		o.LastError = ""
		if err != nil {
			o.LastError = "verification_or_reconciliation_failed"
		}
		if saveErr := s.repo.SaveObservation(ctx, c, o); saveErr != nil {
			return saveErr
		}
		if err != nil {
			return err
		}
	}
	if len(o.Subjects) == 0 || o.LastError != "" {
		return nil
	}
	events, err := s.repo.Events(ctx, c, o)
	if err != nil {
		return err
	}
	if len(events) > 0 {
		if !o.AttemptedAt.IsZero() && time.Since(o.AttemptedAt) < 10*time.Second {
			return nil
		}
		// Recheck authorization and live heads before consuming queued facts. A
		// cached reconciliation snapshot cannot authorize an old-head review.
		err := s.refreshEventSubjects(ctx, resolver, c, &o)
		o.AttemptedAt = time.Now().UTC()
		if err != nil {
			o.LastError = "verification_or_reconciliation_failed"
		}
		if saveErr := s.repo.SaveObservation(ctx, c, o); saveErr != nil {
			return saveErr
		}
		if err != nil {
			return err
		}
	}
	for _, event := range events {
		if !event.OccurredAt.After(o.ActivatedAt) {
			continue
		}
		index := -1
		for i, subject := range o.Subjects {
			if sameSubject(subject, event.Subject) {
				index = i
				break
			}
		}
		if index < 0 {
			continue
		}
		// Matching receives API-verified current state; event-specific state stays
		// in the immutable facts. Providers own head and aggregate conditions.
		event.Subject.HeadSHA = o.Subjects[index].HeadSHA
		matched, e := p.Matches(c.ActionID, c.Parameters, event)
		if e != nil {
			return e
		}
		if !matched {
			continue
		}
		if aggregator, ok := p.(SubjectMatcher); ok {
			decision, matchErr := aggregator.MatchSubjects(c.ActionID, c.Parameters, event, o.Subjects)
			if matchErr != nil {
				return matchErr
			}
			if !decision.Matched {
				continue
			}
			if decision.OncePerObservation {
				event.ID = "aggregate_satisfied"
			}
		}
		if err := s.repo.AdmitEvent(ctx, c, o, event, eventContext(event)); err != nil && !errors.Is(err, problem.ErrConflict) {
			return err
		}
	}
	return nil
}

func sameSubject(a, b Subject) bool {
	return a.Kind == b.Kind && a.ID == b.ID && a.RepositoryID == b.RepositoryID
}

func (s *WorkflowStageIntegrationService) refreshObservation(ctx context.Context, p WorkflowStageIntegration, resolver SubjectProvider, c Candidate, o *Observation, now time.Time) error {
	if err := p.ValidateParameters(c.ActionID, c.Parameters); err != nil {
		return err
	}
	if err := s.authorize(ctx, c.ActorID, c.OrganizationID, c.Connection.WorkspaceID, true); err != nil {
		return err
	}
	credentials, err := s.coding.CodingCredential(ctx, c.Connection.WorkspaceID, c.Connection.CodingIntegrationID)
	if err != nil {
		return err
	}
	if len(o.Subjects) == 0 {
		if len(c.Artifacts) == 0 {
			return problem.ErrInvalid
		}
		o.RevisionIDs = []string{}
		o.Subjects = []Subject{}
		for _, artifact := range c.Artifacts {
			if artifact.Kind != "pull_request" && artifact.Kind != "code_review" {
				return problem.ErrInvalid
			}
			subject, e := resolver.ResolveSubject(ctx, c.Connection.Config, credentials.Token, artifact.Content.URL)
			if e != nil {
				return e
			}
			if !allowedSubject(c, subject) {
				return problem.ErrForbidden
			}
			o.RevisionIDs = append(o.RevisionIDs, artifact.ID)
			duplicate := false
			for _, existing := range o.Subjects {
				if sameSubject(existing, subject) {
					duplicate = true
				}
			}
			if !duplicate {
				o.Subjects = append(o.Subjects, subject)
			}
		}
		// Historical state is a baseline, not a trigger. Set activation only after
		// every selected artifact has been verified successfully.
		o.ActivatedAt = now
		return nil
	}
	for i, subject := range o.Subjects {
		current, e := resolver.ResolveSubject(ctx, c.Connection.Config, credentials.Token, subject.URL)
		if e != nil {
			return e
		}
		if !sameSubject(subject, current) || !allowedSubject(c, current) {
			return problem.ErrForbidden
		}
		o.Subjects[i] = current
		events, e := resolver.Reconcile(ctx, c.Connection.Config, credentials.Token, current, reconciliationStart(*o))
		if e != nil {
			return e
		}
		if err := s.repo.RecordEvents(ctx, c.Connection, events); err != nil {
			return err
		}
	}
	return nil
}

func allowedSubject(c Candidate, subject Subject) bool {
	for _, repository := range c.Repositories {
		if repository.IntegrationID != c.Connection.CodingIntegrationID || repository.ProviderRepositoryID != subject.RepositoryID {
			continue
		}
		for _, id := range c.Connection.RepositoryIDs {
			if id == repository.ID {
				return true
			}
		}
	}
	return false
}

func reconciliationStart(o Observation) time.Time {
	since := o.ReconciledAt.Add(-time.Minute)
	if o.ReconciledAt.IsZero() || since.Before(o.ActivatedAt) {
		return o.ActivatedAt
	}
	return since
}

func (s *WorkflowStageIntegrationService) refreshEventSubjects(ctx context.Context, resolver SubjectProvider, c Candidate, o *Observation) error {
	if err := s.authorize(ctx, c.ActorID, c.OrganizationID, c.Connection.WorkspaceID, true); err != nil {
		return err
	}
	credential, err := s.coding.CodingCredential(ctx, c.Connection.WorkspaceID, c.Connection.CodingIntegrationID)
	if err != nil {
		return err
	}
	for i, subject := range o.Subjects {
		current, err := resolver.ResolveSubject(ctx, c.Connection.Config, credential.Token, subject.URL)
		if err != nil {
			return err
		}
		if !sameSubject(subject, current) || !allowedSubject(c, current) {
			return problem.ErrForbidden
		}
		o.Subjects[i] = current
	}
	return nil
}
