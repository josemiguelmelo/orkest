package pullrequestlink

import "testing"

func TestMatchRequiresUnambiguousRepositoryBranchAndCommit(t *testing.T) {
	pr := PullRequest{RepositoryID: "base", HeadRepositoryID: "head", Branch: "any/user/name", CommitID: "sha"}
	candidate := Candidate{TaskID: "task", RepositoryID: "base", HeadRepositoryID: "head", Branch: pr.Branch, CommitID: "sha"}
	for _, tc := range []struct {
		name             string
		candidates       []Candidate
		task, diagnostic string
	}{
		{"exact", []Candidate{candidate}, "task", ""},
		{"duplicate evidence", []Candidate{candidate, candidate}, "task", ""},
		{"unknown", nil, "", "waiting_for_branch_evidence"},
		{"another task", []Candidate{candidate, {TaskID: "other", RepositoryID: "base", HeadRepositoryID: "head", Branch: pr.Branch, CommitID: "sha"}}, "", "conflicting_branch_ownership"},
		{"wrong head", []Candidate{{TaskID: "task", RepositoryID: "base", HeadRepositoryID: "fork", Branch: pr.Branch, CommitID: "sha"}}, "", "waiting_for_branch_evidence"},
		{"wrong commit", []Candidate{{TaskID: "task", RepositoryID: "base", HeadRepositoryID: "head", Branch: pr.Branch, CommitID: "other"}}, "", "waiting_for_branch_evidence"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task, diagnostic := Match(pr, tc.candidates)
			if task != tc.task || diagnostic != tc.diagnostic {
				t.Fatalf("%s %s", task, diagnostic)
			}
		})
	}
}

func TestBranchEvidenceEligibility(t *testing.T) {
	for _, tc := range []struct {
		name      string
		candidate Candidate
		eligible  bool
	}{
		{"new task branch", Candidate{Branch: "skill/name"}, true},
		{"baseline", Candidate{Branch: "skill/name", Baseline: true}, false},
		{"unavailable", Candidate{Branch: "skill/name", Diagnostic: "checkout_unavailable"}, false},
		{"default branch", Candidate{Branch: "main", DefaultBranch: "main"}, false},
		{"empty", Candidate{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.candidate.CanAssociate() != tc.eligible {
				t.Fatal("incorrect eligibility")
			}
		})
	}
}
