package smtpdebug

import (
	"fmt"
	"sync"
	"time"
)

const (
	DefaultLimit = 1000

	EventListenStart = "listen_start"
	EventListenError = "listen_error"
	EventConnect     = "connect"
	EventHelo        = "helo"
	EventMailFrom    = "mail_from"
	EventRcptAccept  = "rcpt_accept"
	EventRcptReject  = "rcpt_reject"
	EventDataStart   = "data_start"
	EventDataError   = "data_error"
	EventMailStored  = "mail_stored"
)

type Event struct {
	ID         string    `json:"id"`
	Time       time.Time `json:"time"`
	Type       string    `json:"type"`
	SessionID  string    `json:"sessionId,omitempty"`
	RemoteAddr string    `json:"remoteAddr,omitempty"`
	LocalAddr  string    `json:"localAddr,omitempty"`
	Helo       string    `json:"helo,omitempty"`
	From       string    `json:"from,omitempty"`
	To         string    `json:"to,omitempty"`
	Recipients []string  `json:"recipients,omitempty"`
	MessageID  string    `json:"messageId,omitempty"`
	Size       int64     `json:"size,omitempty"`
	Error      string    `json:"error,omitempty"`
	Detail     string    `json:"detail,omitempty"`
}

type Store struct {
	mu     sync.RWMutex
	next   uint64
	limit  int
	events []Event
}

func New(limit int) *Store {
	if limit <= 0 {
		limit = DefaultLimit
	}
	return &Store{limit: limit}
}

func (s *Store) Add(event Event) Event {
	if s == nil {
		return event
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	if event.ID == "" {
		event.ID = fmt.Sprintf("smtp_%d", s.next)
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	} else {
		event.Time = event.Time.UTC()
	}
	event.Recipients = cloneStrings(event.Recipients)
	s.events = append(s.events, event)
	if overflow := len(s.events) - s.limit; overflow > 0 {
		copy(s.events, s.events[overflow:])
		s.events = s.events[:s.limit]
	}
	return event
}

func (s *Store) List() []Event {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Event, 0, len(s.events))
	for i := len(s.events) - 1; i >= 0; i-- {
		event := s.events[i]
		event.Recipients = cloneStrings(event.Recipients)
		items = append(items, event)
	}
	return items
}

func (s *Store) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = nil
}

func cloneStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, len(items))
	copy(out, items)
	return out
}
