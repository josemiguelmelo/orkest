package agentctl

const subscriberBuffer = 64

// EventSubscriptions manages bounded live delivery. All methods require ConversationService.mu.
// Slow subscribers are detached without blocking execution.
type EventSubscriptions struct{ channels map[chan Event]struct{} }

func (s *EventSubscriptions) Add() chan Event {
	if s.channels == nil {
		s.channels = make(map[chan Event]struct{})
	}
	channel := make(chan Event, subscriberBuffer)
	s.channels[channel] = struct{}{}
	return channel
}

func (s *EventSubscriptions) Remove(channel chan Event) {
	if _, ok := s.channels[channel]; ok {
		delete(s.channels, channel)
		close(channel)
	}
}

func (s *EventSubscriptions) Publish(event Event) {
	for channel := range s.channels {
		select {
		case channel <- event:
		default:
			s.Remove(channel)
		}
	}
}

func (s *EventSubscriptions) Close() {
	for channel := range s.channels {
		s.Remove(channel)
	}
}
