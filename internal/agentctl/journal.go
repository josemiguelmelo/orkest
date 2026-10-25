package agentctl

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	maxReplayEvents = 4096
	maxEventBytes   = 1024 * 1024
	maxReplayBytes  = 8 * 1024 * 1024
)

var (
	ErrCursor  = errors.New("event cursor is outside the retained replay window")
	ErrJournal = errors.New("conversation journal unavailable")
)

// durableFile is the journal's local I/O contract, allowing deterministic write
// and sync failure tests without exposing file handles to conversation code.
type durableFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

// Journal owns durable JSONL storage and a bounded replay window. ConversationService.mu
// serializes access; an append failure permanently fences further appends.
// Old records remain on disk, but are not loaded automatically after restart.
type Journal struct {
	file     durableFile
	failed   bool
	closed   bool
	sequence uint64
	events   []Event
	sizes    []int
	bytes    int
}

func openJournal(stateDir string, config RunConfig) (*Journal, error) {
	if !filepath.IsAbs(stateDir) {
		return nil, fmt.Errorf("%w: absolute state directory required", ErrJournal)
	}
	scope, _ := json.Marshal([]string{config.OrganizationID, config.WorkspaceID, config.TaskID, config.RunID})
	dir := filepath.Join(stateDir, "conversations")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("%w: create conversation storage failed", ErrJournal)
	}
	file, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%x.jsonl", sha256.Sum256(scope))), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("%w: conversation history exists or is unavailable; reconciliation required", ErrJournal)
	}
	return &Journal{file: file}, nil
}

func (j *Journal) Append(event Event) error {
	if j.failed || j.closed {
		return ErrJournal
	}
	data, err := json.Marshal(event)
	if err == nil && len(data) > maxEventBytes {
		err = ErrJournal
	}
	if err == nil {
		var written int
		written, err = j.file.Write(append(data, '\n'))
		if err == nil && written != len(data)+1 {
			err = io.ErrShortWrite
		}
	}
	if err == nil {
		err = j.file.Sync()
	}
	if err != nil {
		j.failed = true
		return ErrJournal
	}
	j.sequence = event.Sequence
	j.events = append(j.events, event)
	j.sizes = append(j.sizes, len(data))
	j.bytes += len(data)
	for len(j.events) > maxReplayEvents || j.bytes > maxReplayBytes {
		j.bytes -= j.sizes[0]
		j.events[0] = Event{}
		j.events = j.events[1:]
		j.sizes = j.sizes[1:]
	}
	return nil
}

func (j *Journal) Replay(after uint64) ([]Event, error) {
	if j == nil {
		if after != 0 {
			return nil, ErrCursor
		}
		return nil, nil
	}
	first := j.sequence - uint64(len(j.events))
	if after < first || after > j.sequence {
		return nil, ErrCursor
	}
	return append([]Event(nil), j.events[after-first:]...), nil
}

func (j *Journal) Close() error {
	if j.closed {
		return nil
	}
	j.closed = true
	return j.file.Close()
}
