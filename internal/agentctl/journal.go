package agentctl

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	maxReplayEvents = 4096
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

// Journal owns sequence allocation, durable JSONL storage, and a bounded replay
// window. Its methods are safe for concurrent use; append failures fence writes.
// Old records remain on disk, but are not loaded automatically after restart.
type Journal struct {
	path       string
	diskReplay bool
	mu         sync.Mutex
	file       durableFile
	failed     bool
	closed     bool
	sequence   uint64
	events     []Event
}

func openJournal(stateDir string, config RunConfig) (*Journal, error) {
	journal, _, err := loadJournal(stateDir, config, false)
	return journal, err
}

func loadJournal(stateDir string, config RunConfig, restore bool) (*Journal, []Event, error) {
	if !filepath.IsAbs(stateDir) {
		return nil, nil, fmt.Errorf("%w: absolute state directory required", ErrJournal)
	}
	scope, _ := json.Marshal([]string{config.OrganizationID, config.WorkspaceID, config.TaskID, config.RunID})
	dir := filepath.Join(stateDir, "conversations")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("%w: create conversation storage failed", ErrJournal)
	}
	path := filepath.Join(dir, fmt.Sprintf("%x.jsonl", sha256.Sum256(scope)))
	flags := os.O_RDWR | os.O_CREATE | os.O_EXCL
	if restore {
		flags = os.O_RDWR
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if restore && errors.Is(err, os.ErrNotExist) {
		return loadJournal(stateDir, config, false)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("%w: conversation history exists or is unavailable; reconciliation required", ErrJournal)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, nil, ErrJournal
	}
	journal := &Journal{file: file, path: path, diskReplay: config.TaskSessionID != ""}
	var history []Event
	if restore {
		reader := bufio.NewReader(file)
		var offset int64
		for {
			line, readErr := reader.ReadBytes('\n')
			if errors.Is(readErr, io.EOF) {
				if len(line) > 0 {
					if err := file.Truncate(offset); err != nil {
						_ = file.Close()
						return nil, nil, ErrJournal
					}
				}
				break
			}
			if readErr != nil {
				_ = file.Close()
				return nil, nil, ErrJournal
			}
			var event Event
			if len(bytes.TrimSpace(line)) == 0 || json.Unmarshal(line, &event) != nil || event.Sequence != journal.sequence+1 || event.OrganizationID != config.OrganizationID || event.WorkspaceID != config.WorkspaceID || event.TaskID != config.TaskID || event.RunID != config.RunID {
				_ = file.Close()
				return nil, nil, ErrJournal
			}
			offset += int64(len(line))
			journal.sequence = event.Sequence
			history = append(history, event)
		}
		if _, err := file.Seek(0, io.SeekEnd); err != nil {
			_ = file.Close()
			return nil, nil, ErrJournal
		}
		start := max(0, len(history)-maxReplayEvents)
		journal.events = append([]Event(nil), history[start:]...)
	}
	return journal, history, nil
}

// Append assigns commit metadata and returns the event only after it is synced.
func (j *Journal) Append(event Event) (Event, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.failed || j.closed {
		return Event{}, ErrJournal
	}
	event.Sequence = j.sequence + 1
	event.At = time.Now().UTC()
	data, err := json.Marshal(event)
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
		return Event{}, ErrJournal
	}
	j.sequence = event.Sequence
	j.events = append(j.events, event)
	for len(j.events) > maxReplayEvents {
		j.events[0] = Event{}
		j.events = j.events[1:]
	}
	return event, nil
}

func (j *Journal) Replay(after uint64) ([]Event, error) {
	if j == nil {
		if after != 0 {
			return nil, ErrCursor
		}
		return nil, nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	first := j.sequence - uint64(len(j.events))
	if after > j.sequence {
		return nil, ErrCursor
	}
	if after < first {
		if j.diskReplay {
			return j.replayDisk(after)
		}
		return nil, ErrCursor
	}
	return append([]Event(nil), j.events[after-first:]...), nil
}

func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	return j.file.Close()
}

// replayDisk is called with the journal lock held, so publication cannot race
// snapshotting older committed history during a reconnect.
func (j *Journal) replayDisk(after uint64) ([]Event, error) {
	file, err := os.Open(j.path)
	if err != nil {
		return nil, ErrJournal
	}
	defer func() { _ = file.Close() }()
	reader := bufio.NewReader(file)
	var events []Event
	for expected := uint64(1); expected <= j.sequence; expected++ {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return nil, ErrJournal
		}
		var event Event
		if json.Unmarshal(line, &event) != nil || event.Sequence != expected {
			return nil, ErrJournal
		}
		if event.Sequence > after {
			events = append(events, event)
		}
	}
	return events, nil
}
