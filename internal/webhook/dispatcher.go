package webhook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PikachuCN/LeMail/internal/config"
	"github.com/PikachuCN/LeMail/internal/mailstore"
)

type Dispatcher struct {
	manager *config.Manager
	client  *http.Client
	logger  *slog.Logger
}

type Payload struct {
	Event      string    `json:"event"`
	RuleID     string    `json:"ruleId"`
	Mailbox    string    `json:"mailbox"`
	Code       string    `json:"code"`
	Subject    string    `json:"subject"`
	From       string    `json:"from"`
	ReceivedAt time.Time `json:"receivedAt"`
	MessageID  string    `json:"messageId"`
}

func NewDispatcher(manager *config.Manager, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		manager: manager,
		client:  &http.Client{Timeout: 5 * time.Second},
		logger:  logger,
	}
}

func (d *Dispatcher) HandleMessage(msg mailstore.Message) {
	cfg := d.manager.Get()
	for _, rule := range cfg.Webhooks {
		if !rule.Enabled {
			continue
		}
		if err := ValidateRule(rule); err != nil {
			d.logger.Warn("skip invalid webhook rule", "rule", rule.ID, "error", err)
			continue
		}
		for _, mailbox := range msg.To {
			if !matchesMailbox(rule, mailbox) || !matchesMessage(rule, msg) {
				continue
			}
			code, ok := extractCode(rule, msg)
			if !ok {
				continue
			}
			payload := Payload{
				Event:      "verification_code",
				RuleID:     rule.ID,
				Mailbox:    mailbox,
				Code:       code,
				Subject:    msg.Subject,
				From:       msg.From,
				ReceivedAt: msg.ReceivedAt,
				MessageID:  msg.ID,
			}
			go d.post(rule, payload)
		}
	}
}

func ValidateRule(rule config.WebhookRule) error {
	if strings.TrimSpace(rule.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(rule.URL) == "" {
		return errors.New("url is required")
	}
	parsed, err := url.Parse(rule.URL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("url must be absolute")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("url must use http or https")
	}
	if strings.TrimSpace(rule.CodePattern) == "" {
		return errors.New("codePattern is required")
	}
	if _, err := regexp.Compile(rule.CodePattern); err != nil {
		return fmt.Errorf("codePattern: %w", err)
	}
	if strings.TrimSpace(rule.FromPattern) != "" {
		if _, err := regexp.Compile(rule.FromPattern); err != nil {
			return fmt.Errorf("fromPattern: %w", err)
		}
	}
	source := strings.ToLower(strings.TrimSpace(rule.Source))
	if source == "" {
		return nil
	}
	switch source {
	case "text", "html", "raw", "all":
		return nil
	default:
		return errors.New("source must be text, html, raw, or all")
	}
}

func (d *Dispatcher) post(rule config.WebhookRule, payload Payload) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, rule.URL, bytes.NewReader(body))
	if err != nil {
		d.logger.Warn("create webhook request failed", "rule", rule.ID, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		d.logger.Warn("webhook delivery failed", "rule", rule.ID, "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		d.logger.Warn("webhook delivery returned non-2xx", "rule", rule.ID, "status", resp.StatusCode)
	}
}

func matchesMailbox(rule config.WebhookRule, mailbox string) bool {
	local, domain, ok := strings.Cut(strings.ToLower(mailbox), "@")
	if !ok {
		return false
	}
	if len(rule.Domains) > 0 && !containsFold(rule.Domains, domain) {
		return false
	}
	if len(rule.LocalParts) > 0 && !containsFold(rule.LocalParts, local) {
		return false
	}
	return true
}

func matchesMessage(rule config.WebhookRule, msg mailstore.Message) bool {
	if strings.TrimSpace(rule.FromPattern) != "" {
		matched, err := regexp.MatchString(rule.FromPattern, msg.From)
		if err != nil || !matched {
			return false
		}
	}
	if strings.TrimSpace(rule.Subject) != "" && !strings.Contains(strings.ToLower(msg.Subject), strings.ToLower(rule.Subject)) {
		return false
	}
	return true
}

func extractCode(rule config.WebhookRule, msg mailstore.Message) (string, bool) {
	re := regexp.MustCompile(rule.CodePattern)
	for _, candidate := range sourceCandidates(rule.Source, msg) {
		matches := re.FindStringSubmatch(candidate)
		if len(matches) == 0 {
			continue
		}
		if len(matches) > 1 {
			return matches[1], true
		}
		return matches[0], true
	}
	return "", false
}

func sourceCandidates(source string, msg mailstore.Message) []string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "text":
		return []string{msg.Text}
	case "html":
		return []string{msg.HTML}
	case "raw":
		return []string{msg.Raw}
	default:
		return []string{msg.Text, msg.HTML, msg.Raw}
	}
}

func containsFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}
