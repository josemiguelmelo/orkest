package agentctl

import (
	"errors"
	"os"
	"strings"
	"testing"
)

type failingFile struct{ syncFailure bool }

func (f failingFile) Write(data []byte) (int, error) {
	if !f.syncFailure {
		return 0, errors.New("disk write failed")
	}
	return len(data), nil
}
func (f failingFile) Sync() error  { return errors.New("disk sync failed") }
func (f failingFile) Close() error { return nil }

func TestJournalFailureDoesNotPublishOrAdvance(t *testing.T) {
	for _, syncFailure := range []bool{false, true} {
		journal := &Journal{file: failingFile{syncFailure: syncFailure}}
		if err := journal.Append(Event{Sequence: 1}); !errors.Is(err, ErrJournal) {
			t.Fatalf("append: %v", err)
		}
		events, err := journal.Replay(0)
		if err != nil || len(events) != 0 || journal.sequence != 0 {
			t.Fatal("failed append advanced replay")
		}
		if err := journal.Append(Event{Sequence: 1}); !errors.Is(err, ErrJournal) {
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
	// The byte budget expires before the event count budget in this case.
	text := strings.Repeat("x", maxEventBytes/2)
	for sequence := uint64(1); sequence <= 20; sequence++ {
		if err := journal.Append(Event{Sequence: sequence, Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := journal.Replay(0); !errors.Is(err, ErrCursor) {
		t.Fatal("expired cursor accepted")
	}
	if _, err := journal.Replay(21); !errors.Is(err, ErrCursor) {
		t.Fatal("future cursor accepted")
	}
	events, err := journal.Replay(19)
	if err != nil || len(events) != 1 || events[0].Sequence != 20 {
		t.Fatalf("replay: %v, %v", len(events), err)
	}
	if journal.bytes > maxReplayBytes {
		t.Fatal("replay byte limit exceeded")
	}
	// Full history remains durable after it leaves the replay window.
	info, err := journal.file.(*os.File).Stat()
	if err != nil || info.Size() <= maxReplayBytes {
		t.Fatal("durable history was truncated", err)
	}
}

type successfulFile struct{}

func (successfulFile) Write(data []byte) (int, error) { return len(data), nil }
func (successfulFile) Sync() error                    { return nil }
func (successfulFile) Close() error                   { return nil }

func TestJournalReplayEventLimit(t *testing.T) {
	journal := &Journal{file: successfulFile{}}
	for sequence := uint64(1); sequence <= maxReplayEvents+1; sequence++ {
		if err := journal.Append(Event{Sequence: sequence}); err != nil {
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

func TestJournalOversizedEventInterruptsAppend(t *testing.T) {
	journal := &Journal{file: successfulFile{}}
	if err := journal.Append(Event{Sequence: 1, Text: strings.Repeat("x", maxEventBytes)}); !errors.Is(err, ErrJournal) {
		t.Fatal("oversized event accepted", err)
	}
	if journal.sequence != 0 {
		t.Fatal("oversized event advanced cursor")
	}
}
