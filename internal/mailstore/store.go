package mailstore

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	stdmail "net/mail"
	"net/textproto"
	"sort"
	"strings"
	"sync"
	"time"
)

type Message struct {
	ID         string              `json:"id"`
	From       string              `json:"from"`
	To         []string            `json:"to"`
	Subject    string              `json:"subject"`
	Text       string              `json:"text"`
	HTML       string              `json:"html"`
	Headers    map[string][]string `json:"headers"`
	Raw        string              `json:"raw,omitempty"`
	ReceivedAt time.Time           `json:"receivedAt"`
}

type Store struct {
	mu      sync.RWMutex
	byID    map[string]Message
	mailbox map[string][]string
}

func New() *Store {
	return &Store{byID: make(map[string]Message), mailbox: make(map[string][]string)}
}

func NewMessage(raw []byte, envelopeFrom string, recipients []string, receivedAt time.Time) Message {
	msg := Message{
		ID:         randomID(),
		From:       envelopeFrom,
		To:         normalizeRecipients(recipients),
		Headers:    map[string][]string{},
		Raw:        string(raw),
		ReceivedAt: receivedAt.UTC(),
	}
	if parsed, err := stdmail.ReadMessage(bytes.NewReader(raw)); err == nil {
		for key, values := range parsed.Header {
			copied := make([]string, len(values))
			copy(copied, values)
			msg.Headers[key] = copied
		}
		decoder := new(mime.WordDecoder)
		msg.Subject = decodeHeader(decoder, parsed.Header.Get("Subject"))
		if from := decodeHeader(decoder, parsed.Header.Get("From")); strings.TrimSpace(from) != "" {
			msg.From = from
		}
		collectBody(textproto.MIMEHeader(parsed.Header), parsed.Body, &msg.Text, &msg.HTML)
	}
	return msg
}

func (s *Store) Add(msg Message) Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	if msg.ID == "" {
		msg.ID = randomID()
	}
	if msg.ReceivedAt.IsZero() {
		msg.ReceivedAt = time.Now().UTC()
	}
	msg.To = normalizeRecipients(msg.To)
	s.byID[msg.ID] = msg
	for _, address := range msg.To {
		s.mailbox[address] = append(s.mailbox[address], msg.ID)
	}
	return msg
}

func (s *Store) ListMailbox(address string) []Message {
	address = normalizeAddress(address)
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.mailbox[address]
	items := make([]Message, 0, len(ids))
	for _, id := range ids {
		if msg, ok := s.byID[id]; ok {
			items = append(items, msg)
		}
	}
	sortMessages(items)
	return items
}

func (s *Store) Get(id string) (Message, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msg, ok := s.byID[id]
	return msg, ok
}

func (s *Store) ListAll() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Message, 0, len(s.byID))
	for _, msg := range s.byID {
		items = append(items, msg)
	}
	sortMessages(items)
	return items
}

func (s *Store) Cleanup(now time.Time, retention time.Duration) int {
	if retention <= 0 {
		retention = time.Hour
	}
	cutoff := now.UTC().Add(-retention)
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for id, msg := range s.byID {
		if msg.ReceivedAt.Before(cutoff) {
			delete(s.byID, id)
			removed++
		}
	}
	if removed > 0 {
		s.rebuildIndexesLocked()
	}
	return removed
}

func (s *Store) StartJanitor(stop <-chan struct{}, retention func() time.Duration) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.Cleanup(time.Now(), retention())
		case <-stop:
			return
		}
	}
}

func (s *Store) rebuildIndexesLocked() {
	s.mailbox = make(map[string][]string)
	for id, msg := range s.byID {
		for _, address := range msg.To {
			s.mailbox[address] = append(s.mailbox[address], id)
		}
	}
}

func collectBody(header textproto.MIMEHeader, body io.Reader, textOut *string, htmlOut *string) {
	mediaType, params, _ := mime.ParseMediaType(header.Get("Content-Type"))
	if strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return
		}
		reader := multipart.NewReader(body, boundary)
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return
			}
			collectBody(part.Header, part, textOut, htmlOut)
		}
		return
	}
	data, err := io.ReadAll(decodeTransfer(header, body))
	if err != nil {
		return
	}
	switch strings.ToLower(mediaType) {
	case "text/html":
		if *htmlOut == "" {
			*htmlOut = string(data)
		}
	case "text/plain", "":
		if *textOut == "" {
			*textOut = string(data)
		}
	}
}

func decodeTransfer(header textproto.MIMEHeader, body io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding"))) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, body)
	case "quoted-printable":
		return quotedprintable.NewReader(body)
	default:
		return body
	}
}

func decodeHeader(decoder *mime.WordDecoder, value string) string {
	decoded, err := decoder.DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

func normalizeRecipients(recipients []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		address := normalizeAddress(recipient)
		if address == "" {
			continue
		}
		if _, ok := seen[address]; ok {
			continue
		}
		seen[address] = struct{}{}
		out = append(out, address)
	}
	return out
}

func normalizeAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(strings.Trim(address, "<>")))
}

func sortMessages(items []Message) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].ReceivedAt.After(items[j].ReceivedAt)
	})
}

func randomID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err == nil {
		return hex.EncodeToString(data[:])
	}
	return strings.ReplaceAll(time.Now().UTC().Format(time.RFC3339Nano), ":", "")
}
