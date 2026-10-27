package run

import (
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
)

const subscriberBuffer = 64

// eventSubscriptions manages bounded live delivery. All methods require conversation.mu.
// Slow subscribers are detached without blocking execution.
type eventSubscriptions struct {
	channels map[chan interaction.Event]struct{}
}

func (s *eventSubscriptions) Add() chan interaction.Event {
	if s.channels == nil {
		s.channels = make(map[chan interaction.Event]struct{})
	}
	channel := make(chan interaction.Event, subscriberBuffer)
	s.channels[channel] = struct{}{}
	return channel
}

func (s *eventSubscriptions) Remove(channel chan interaction.Event) {
	if _, ok := s.channels[channel]; ok {
		delete(s.channels, channel)
		close(channel)
	}
}

func (s *eventSubscriptions) Publish(event interaction.Event) {
	for channel := range s.channels {
		select {
		case channel <- event:
		default:
			s.Remove(channel)
		}
	}
}

func (s *eventSubscriptions) Close() {
	for channel := range s.channels {
		s.Remove(channel)
	}
}
