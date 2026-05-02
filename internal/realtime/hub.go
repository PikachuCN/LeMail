package realtime

import (
	"sync"

	"github.com/PikachuCN/LeMail/internal/mailstore"
)

type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]map[*Subscription]struct{}
}

type Subscription struct {
	address string
	hub     *Hub
	C       chan mailstore.Message
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[string]map[*Subscription]struct{})}
}

func (h *Hub) Subscribe(address string) *Subscription {
	sub := &Subscription{address: normalizeAddress(address), hub: h, C: make(chan mailstore.Message, 16)}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subscribers[sub.address] == nil {
		h.subscribers[sub.address] = make(map[*Subscription]struct{})
	}
	h.subscribers[sub.address][sub] = struct{}{}
	return sub
}

func (h *Hub) Publish(address string, msg mailstore.Message) {
	address = normalizeAddress(address)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for sub := range h.subscribers[address] {
		select {
		case sub.C <- msg:
		default:
		}
	}
}

func (s *Subscription) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	if subscribers := s.hub.subscribers[s.address]; subscribers != nil {
		delete(subscribers, s)
		if len(subscribers) == 0 {
			delete(s.hub.subscribers, s.address)
		}
	}
	close(s.C)
}

func normalizeAddress(address string) string {
	return stringsToLowerTrim(address)
}

func stringsToLowerTrim(value string) string {
	b := []byte(value)
	start, end := 0, len(b)
	for start < end && (b[start] == ' ' || b[start] == '\t' || b[start] == '\n' || b[start] == '\r') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\n' || b[end-1] == '\r') {
		end--
	}
	for i := start; i < end; i++ {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b[start:end])
}
