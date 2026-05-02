package codeextract

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PikachuCN/LeMail/internal/config"
	"github.com/PikachuCN/LeMail/internal/mailstore"
)

type Match struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"projectId"`
	ProjectName string    `json:"projectName"`
	Mailbox     string    `json:"mailbox"`
	Code        string    `json:"code"`
	Subject     string    `json:"subject"`
	From        string    `json:"from"`
	ReceivedAt  time.Time `json:"receivedAt"`
	MessageID   string    `json:"messageId"`
}

type Store struct {
	mu      sync.RWMutex
	byID    map[string]Match
	mailbox map[string][]string
}

type Processor struct {
	manager *config.Manager
	store   *Store
	logger  *slog.Logger
}

func NewStore() *Store {
	return &Store{byID: make(map[string]Match), mailbox: make(map[string][]string)}
}

func NewProcessor(manager *config.Manager, store *Store, logger *slog.Logger) *Processor {
	return &Processor{manager: manager, store: store, logger: logger}
}

func (p *Processor) HandleMessage(msg mailstore.Message) {
	matches := ExtractMatches(p.manager.Get().CodeProjects, msg, p.logger)
	p.store.AddMany(matches)
}

func (s *Store) AddMany(matches []Match) {
	if len(matches) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, match := range matches {
		if match.ID == "" {
			match.ID = stableID(match.ProjectID, match.MessageID, match.Mailbox, match.Code)
		}
		if match.ReceivedAt.IsZero() {
			match.ReceivedAt = time.Now().UTC()
		}
		match.Mailbox = normalize(match.Mailbox)
		s.byID[match.ID] = match
	}
	s.rebuildIndexesLocked()
}

func (s *Store) Replace(matches []Match) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID = make(map[string]Match, len(matches))
	for _, match := range matches {
		if match.ID == "" {
			match.ID = stableID(match.ProjectID, match.MessageID, match.Mailbox, match.Code)
		}
		if match.ReceivedAt.IsZero() {
			match.ReceivedAt = time.Now().UTC()
		}
		match.Mailbox = normalize(match.Mailbox)
		s.byID[match.ID] = match
	}
	s.rebuildIndexesLocked()
}

func (s *Store) ListMailbox(address string) []Match {
	address = normalize(address)
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.mailbox[address]
	items := make([]Match, 0, len(ids))
	for _, id := range ids {
		if match, ok := s.byID[id]; ok {
			items = append(items, match)
		}
	}
	sortMatches(items)
	return items
}

func (s *Store) ListAll() []Match {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Match, 0, len(s.byID))
	for _, match := range s.byID {
		items = append(items, match)
	}
	sortMatches(items)
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
	for id, match := range s.byID {
		if match.ReceivedAt.Before(cutoff) {
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

func ValidateProject(project config.CodeProject) error {
	if strings.TrimSpace(project.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(project.CodePattern) == "" {
		return errors.New("codePattern is required")
	}
	if _, err := regexp.Compile(project.CodePattern); err != nil {
		return fmt.Errorf("codePattern: %w", err)
	}
	if strings.TrimSpace(project.FromPattern) != "" {
		if _, err := regexp.Compile(project.FromPattern); err != nil {
			return fmt.Errorf("fromPattern: %w", err)
		}
	}
	switch normalizeSource(project.Source) {
	case "", "text", "html", "raw", "all":
		return nil
	default:
		return errors.New("source must be text, html, raw, or all")
	}
}

func ExtractMatches(projects []config.CodeProject, msg mailstore.Message, logger *slog.Logger) []Match {
	var matches []Match
	for _, project := range projects {
		if !project.Enabled {
			continue
		}
		if err := ValidateProject(project); err != nil {
			if logger != nil {
				logger.Warn("skip invalid code project", "project", project.ID, "error", err)
			}
			continue
		}
		if !matchesMessage(project, msg) {
			continue
		}
		code, ok := extractCode(project, msg)
		if !ok {
			continue
		}
		for _, mailbox := range msg.To {
			if !matchesMailbox(project, mailbox) {
				continue
			}
			match := Match{
				ProjectID:   project.ID,
				ProjectName: project.Name,
				Mailbox:     normalize(mailbox),
				Code:        code,
				Subject:     msg.Subject,
				From:        msg.From,
				ReceivedAt:  msg.ReceivedAt,
				MessageID:   msg.ID,
			}
			match.ID = stableID(match.ProjectID, match.MessageID, match.Mailbox, match.Code)
			matches = append(matches, match)
		}
	}
	return matches
}

func matchesMailbox(project config.CodeProject, mailbox string) bool {
	local, domain, ok := strings.Cut(normalize(mailbox), "@")
	if !ok {
		return false
	}
	if len(project.Domains) > 0 && !containsFold(project.Domains, domain) {
		return false
	}
	if len(project.LocalParts) > 0 && !containsFold(project.LocalParts, local) {
		return false
	}
	return true
}

func matchesMessage(project config.CodeProject, msg mailstore.Message) bool {
	if strings.TrimSpace(project.FromPattern) != "" {
		matched, err := regexp.MatchString(project.FromPattern, msg.From)
		if err != nil || !matched {
			return false
		}
	}
	if strings.TrimSpace(project.Subject) != "" && !strings.Contains(strings.ToLower(msg.Subject), strings.ToLower(project.Subject)) {
		return false
	}
	return true
}

func extractCode(project config.CodeProject, msg mailstore.Message) (string, bool) {
	re := regexp.MustCompile(project.CodePattern)
	for _, candidate := range sourceCandidates(project.Source, msg) {
		matches := re.FindStringSubmatch(candidate)
		if len(matches) == 0 {
			continue
		}
		for _, group := range matches[1:] {
			if code := strings.TrimSpace(group); code != "" {
				return code, true
			}
		}
		return strings.TrimSpace(matches[0]), true
	}
	return "", false
}

func sourceCandidates(source string, msg mailstore.Message) []string {
	switch normalizeSource(source) {
	case "text":
		return []string{msg.Text}
	case "html":
		return []string{msg.HTML, htmlToText(msg.HTML)}
	case "raw":
		return []string{msg.Raw}
	default:
		return []string{msg.Text, msg.HTML, htmlToText(msg.HTML), msg.Raw}
	}
}

func normalizeSource(source string) string {
	return strings.ToLower(strings.TrimSpace(source))
}

func containsFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func htmlToText(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	text := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(value, " ")
	text = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`(?s)<[^>]+>`).ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	return strings.Join(strings.Fields(text), " ")
}

func stableID(parts ...string) string {
	h := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return "code_" + hex.EncodeToString(h[:10])
}

func (s *Store) rebuildIndexesLocked() {
	s.mailbox = make(map[string][]string)
	for id, match := range s.byID {
		s.mailbox[match.Mailbox] = append(s.mailbox[match.Mailbox], id)
	}
}

func sortMatches(items []Match) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].ReceivedAt.After(items[j-1].ReceivedAt); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
