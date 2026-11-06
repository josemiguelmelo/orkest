package model

import "testing"

func TestRunConfigCloneOwnsArtifactRevisionIDs(t *testing.T) {
	original := RunConfig{ArtifactRevisionIDs: []string{"revision-1"}}
	cloned := original.Clone()
	cloned.ArtifactRevisionIDs[0] = "revision-2"
	if original.ArtifactRevisionIDs[0] != "revision-1" {
		t.Fatal("mutating cloned artifact revisions changed the original")
	}
	original.ArtifactRevisionIDs[0] = "revision-3"
	if cloned.ArtifactRevisionIDs[0] != "revision-2" {
		t.Fatal("mutating original artifact revisions changed the clone")
	}
}
