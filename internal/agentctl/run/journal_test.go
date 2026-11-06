package run

import (
	"errors"
	"os"
	"sync"
	"testing"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
)

type failingFile struct{ syncFailure bool }

func (f failingFile) Write(data []byte) (int, error) {
	if !f.syncFailure {
		return 0, errors.New("disk write failed")
	}
	return len(data), nil
}

func (f failingFile) Sync() error { return errors.New("disk sync failed") }

func (f failingFile) Close() error { return nil }

func TestJournalFailureDoesNotPublishOrAdvance(t *testing.T) {
	for _, syncFailure := range []bool{false, true} {
		journal := &Journal{file: failingFile{syncFailure: syncFailure}}
		if _, err := journal.Append(interaction.Event{Sequence: 1}); !errors.Is(err, ErrJournal) {
			t.Fatalf("append: %v", err)
		}
		events, err := journal.Replay(0)
		if err != nil || len(events) != 0 || journal.sequence != 0 {
			t.Fatal("failed append advanced replay")
		}
		if _, err := journal.Append(interaction.Event{Sequence: 1}); !errors.Is(err, ErrJournal) {
			t.Fatal("failed journal was reused")
		}
	}
}

func TestJournalReplayWindow(t *testing.T) {
	journal, err := openJournal(t.TempDir(), runConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = journal.Close() }()
	for sequence := uint64(1); sequence <= maxReplayEvents+1; sequence++ {
		if _, err := journal.Append(interaction.Event{Sequence: sequence}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := journal.Replay(0); !errors.Is(err, ErrCursor) {
		t.Fatal("expired cursor accepted")
	}
	if _, err := journal.Replay(maxReplayEvents + 2); !errors.Is(err, ErrCursor) {
		t.Fatal("future cursor accepted")
	}
	events, err := journal.Replay(maxReplayEvents)
	if err != nil || len(events) != 1 || events[0].Sequence != maxReplayEvents+1 {
		t.Fatalf("replay: %v, %v", len(events), err)
	}
	// Full history remains durable after it leaves the replay window.
	info, err := journal.file.(*os.File).Stat()
	if err != nil || info.Size() <= int64(maxReplayEvents) {
		t.Fatal("durable history was truncated", err)
	}
}

type successfulFile struct{}

func (successfulFile) Write(data []byte) (int, error) { return len(data), nil }

func (successfulFile) Sync() error { return nil }

func (successfulFile) Close() error { return nil }

func TestJournalReplayEventLimit(t *testing.T) {
	journal := &Journal{file: successfulFile{}}
	for sequence := uint64(1); sequence <= maxReplayEvents+1; sequence++ {
		if _, err := journal.Append(interaction.Event{Sequence: sequence}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := journal.Replay(0); !errors.Is(err, ErrCursor) {
		t.Fatal("expired cursor accepted")
	}
	events, err := journal.Replay(1)
	if err != nil || len(events) != maxReplayEvents || events[0].Sequence != 2 {
		t.Fatalf("unexpected replay: %d, %v", len(events), err)
	}
}

func TestJournalAcceptsLargeEvent(t *testing.T) {
	journal := &Journal{file: successfulFile{}}
	text := string(make([]byte, 1024*1024+1))
	if _, err := journal.Append(interaction.Event{Sequence: 1, Text: text}); err != nil {
		t.Fatal("large event rejected", err)
	}
	events, err := journal.Replay(0)
	if err != nil || len(events) != 1 || len(events[0].Text) != len(text) {
		t.Fatal("large event missing from replay", err)
	}
}

func TestJournalAssignsOrderedCommitMetadata(t *testing.T) {
	journal := &Journal{file: successfulFile{}}
	const count = 32
	var work sync.WaitGroup
	for range count {
		work.Go(func() {
			// Supplied sequence/timestamp must not override journal ownership.
			event, err := journal.Append(interaction.Event{Sequence: 999})
			if err != nil {
				t.Error(err)
				return
			}
			if event.At.IsZero() || event.Sequence == 999 {
				t.Error("missing commit metadata")
			}
		})
	}
	work.Wait()
	events, err := journal.Replay(0)
	if err != nil || len(events) != count {
		t.Fatalf("replay: %d %v", len(events), err)
	}
	for i, event := range events {
		if event.Sequence != uint64(i+1) {
			t.Fatalf("unordered sequence: %d", event.Sequence)
		}
	}
}

func openJournal(stateDir string, config configmodel.RunConfig) (*Journal, error) {
	journal, _, err := loadJournal(stateDir, config, false)
	return journal, err
}
