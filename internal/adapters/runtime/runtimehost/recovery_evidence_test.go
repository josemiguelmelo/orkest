package runtimehost

import (
	"sync"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func TestHistoryEvidenceRequiresQualifiedCoverage(t *testing.T) {
	command := interaction.Command{ID: "command-1", Text: "continue"}
	for _, coverageVerified := range []bool{false, true} {
		collector := newHistoryCollector("session-1", coverageVerified)
		collector.Add(agentruntime.HistoryObservation{Kind: agentruntime.HistoryUserChunk, Text: interaction.EncodeCommandPrompt(command.ID, command.Text)})
		collector.Add(agentruntime.HistoryObservation{Kind: agentruntime.HistoryReplayDone})
		decision := collector.Decide(command)
		if coverageVerified && decision.Action != agentruntime.RecoveryContinue {
			t.Fatalf("qualified evidence was not accepted: %+v", decision)
		}
		if !coverageVerified && decision.Action != agentruntime.RecoveryPause {
			t.Fatalf("unqualified evidence was accepted: %+v", decision)
		}
	}
}

func TestHistoryCollectorPausesOnIncompleteOrUnsupportedReplay(t *testing.T) {
	command := interaction.Command{ID: "command-1", Text: "continue"}
	matching := agentruntime.HistoryObservation{Kind: agentruntime.HistoryUserChunk, Text: interaction.EncodeCommandPrompt(command.ID, command.Text)}
	for _, observations := range [][]agentruntime.HistoryObservation{
		{matching},
		{matching, {Kind: agentruntime.HistoryUnknown}, {Kind: agentruntime.HistoryReplayDone}},
		{matching, {Kind: agentruntime.HistoryIncomplete}, {Kind: agentruntime.HistoryReplayDone}},
	} {
		collector := newHistoryCollector("session", true)
		for _, observation := range observations {
			collector.Add(observation)
		}
		if decision := collector.Decide(command); decision.Action != agentruntime.RecoveryPause {
			t.Fatalf("uncertain replay was continued: %+v", decision)
		}
	}
}

func TestSealedReplayIgnoresConcurrentLiveObservations(t *testing.T) {
	callbacks := &sessionCallbacks{}
	command := interaction.Command{ID: "checkpoint", Text: "continue"}
	callbacks.beginReplay("session", true)
	callbacks.Observation(agentruntime.HistoryObservation{Kind: agentruntime.HistoryUserChunk, Text: interaction.EncodeCommandPrompt(command.ID, command.Text)})
	if got := callbacks.recoveryDecision(command); got.Action != agentruntime.RecoveryPause {
		t.Fatal("incomplete replay was accepted")
	}
	callbacks.finishReplay()
	var work sync.WaitGroup
	for range 20 {
		work.Go(func() {
			callbacks.Observation(agentruntime.HistoryObservation{Kind: agentruntime.HistoryToolActivity})
			callbacks.Observation(agentruntime.HistoryObservation{Kind: agentruntime.HistoryUnknown})
			if got := callbacks.recoveryDecision(command); got.Action != agentruntime.RecoveryContinue {
				t.Errorf("live activity changed sealed evidence: %+v", got)
			}
		})
	}
	work.Wait()
}

func TestRecoveryEvidenceSeparatesIdenticalUserTurns(t *testing.T) {
	collector := newHistoryCollector("session", true)
	collector.Add(agentruntime.HistoryObservation{Kind: agentruntime.HistoryUserChunk, Text: interaction.EncodeCommandPrompt("first", "continue")})
	collector.Add(agentruntime.HistoryObservation{Kind: agentruntime.HistoryReplayDone})
	if collector.Decide(interaction.Command{ID: "first", Text: "continue"}).Action != agentruntime.RecoveryContinue {
		t.Fatal("matching checkpoint was not recognized")
	}
	if collector.Decide(interaction.Command{ID: "second", Text: "continue"}).Action == agentruntime.RecoveryContinue {
		t.Fatal("an older identical message was mistaken for the interrupted command")
	}
	withTools := newHistoryCollector("session", true)
	withTools.Add(agentruntime.HistoryObservation{Kind: agentruntime.HistoryUserChunk, Text: interaction.EncodeCommandPrompt("first", "continue")})
	withTools.Add(agentruntime.HistoryObservation{Kind: agentruntime.HistoryToolActivity})
	withTools.Add(agentruntime.HistoryObservation{Kind: agentruntime.HistoryReplayDone})
	if withTools.Decide(interaction.Command{ID: "first", Text: "continue"}).Action == agentruntime.RecoveryContinue {
		t.Fatal("tool effects were ignored during reconciliation")
	}
}
