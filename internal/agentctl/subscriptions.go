package agentctl

const subscriberBuffer = 64

// Subscriptions manages bounded live delivery. All methods require ConversationService.mu.
// Slow subscribers are detached without blocking execution.
type Subscriptions struct{ channels map[chan Event]struct{} }

func (s *Subscriptions) Add() chan Event {
	if s.channels == nil {
		s.channels = make(map[chan Event]struct{})
	}
	channel := make(chan Event, subscriberBuffer)
	s.channels[channel] = struct{}{}
	return channel
}

func (s *Subscriptions) Remove(channel chan Event) {
	if _, ok := s.channels[channel]; ok {
		delete(s.channels, channel)
		close(channel)
	}
}

func (s *Subscriptions) Publish(event Event) {
	for channel := range s.channels {
		select {
		case channel <- event:
		default:
			s.Remove(channel)
		}
	}
}

func (s *Subscriptions) Close() {
	for channel := range s.channels {
		s.Remove(channel)
	}
}
