package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/renameio/v2/maybe"
)

const (
	AccessPublic  = "public"
	AccessPrivate = "private"

	OpenAIAPIAuto            = "auto"
	OpenAIAPIResponses       = "responses"
	OpenAIAPIChatCompletions = "chat_completions"
)

type Config struct {
	Server       ServerConfig  `json:"server"`
	SMTP         SMTPConfig    `json:"smtp"`
	Mail         MailConfig    `json:"mail"`
	Access       AccessConfig  `json:"access"`
	Admin        AdminConfig   `json:"admin"`
	API          APIConfig     `json:"api"`
	OpenAI       OpenAIConfig  `json:"openai"`
	Webhooks     []WebhookRule `json:"webhooks"`
	CodeProjects []CodeProject `json:"codeProjects"`
}

type ServerConfig struct {
	HTTPAddr string `json:"httpAddr"`
}

type SMTPConfig struct {
	Addr string `json:"addr"`
}

type MailConfig struct {
	Domains            []string `json:"domains"`
	Retention          string   `json:"retention"`
	ReservedLocalParts []string `json:"reservedLocalParts"`
}

type AccessConfig struct {
	Mode         string `json:"mode"`
	PasswordHash string `json:"passwordHash"`
}

type AdminConfig struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
}

type APIConfig struct {
	Enabled   bool   `json:"enabled"`
	TokenHash string `json:"tokenHash,omitempty"`
}

type OpenAIConfig struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"apiKey,omitempty"`
	BaseURL string `json:"baseURL"`
	Model   string `json:"model"`
	Timeout string `json:"timeout"`
	APIMode string `json:"apiMode"`
}

type WebhookRule struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	URL         string   `json:"url"`
	Domains     []string `json:"domains,omitempty"`
	LocalParts  []string `json:"localParts,omitempty"`
	FromPattern string   `json:"fromPattern,omitempty"`
	Subject     string   `json:"subject,omitempty"`
	CodePattern string   `json:"codePattern"`
	Source      string   `json:"source"`
}

type CodeProject struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	Description string   `json:"description,omitempty"`
	Domains     []string `json:"domains,omitempty"`
	LocalParts  []string `json:"localParts,omitempty"`
	FromPattern string   `json:"fromPattern,omitempty"`
	Subject     string   `json:"subject,omitempty"`
	CodePattern string   `json:"codePattern"`
	Source      string   `json:"source"`
}

type Manager struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

func DefaultPath() string {
	if path := strings.TrimSpace(os.Getenv("CONFIG_PATH")); path != "" {
		return path
	}
	return filepath.Join("config", "config.json")
}

func LoadManager(path string) (*Manager, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	return &Manager{path: path, cfg: cfg}, nil
}

func Load(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		path = DefaultPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
		fallback := filepath.Join("config", "config.example.json")
		data, err = os.ReadFile(fallback)
		if err != nil {
			return Default(), nil
		}
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Default() Config {
	cfg := Config{
		Server: ServerConfig{HTTPAddr: "0.0.0.0:3000"},
		SMTP:   SMTPConfig{Addr: "0.0.0.0:2525"},
		Mail: MailConfig{
			Domains:   []string{"localhost"},
			Retention: "1h",
			ReservedLocalParts: []string{
				"admin", "postmaster", "system", "webmaster", "administrator", "hostmaster", "service", "server", "root",
			},
		},
		Access: AccessConfig{Mode: AccessPublic},
		Admin:  AdminConfig{Username: "admin"},
		OpenAI: OpenAIConfig{
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-5.4-mini",
			Timeout: "15s",
			APIMode: OpenAIAPIAuto,
		},
	}
	return cfg
}

func (m *Manager) Path() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.path
}

func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return clone(m.cfg)
}

func (m *Manager) Replace(cfg Config) error {
	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = clone(cfg)
	return nil
}

func (m *Manager) Save(cfg Config) error {
	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return err
	}
	if err := maybe.WriteFile(m.path, data, 0o600); err != nil {
		return err
	}
	m.cfg = clone(cfg)
	return nil
}

func (c Config) RetentionDuration() time.Duration {
	d, err := time.ParseDuration(c.Mail.Retention)
	if err != nil || d <= 0 {
		return time.Hour
	}
	return d
}

func (c Config) HasDomain(domain string) bool {
	domain = NormalizeDomain(domain)
	for _, item := range c.Mail.Domains {
		if NormalizeDomain(item) == domain {
			return true
		}
	}
	return false
}

func (c Config) IsReservedLocalPart(local string) bool {
	local = strings.ToLower(strings.TrimSpace(local))
	for _, reserved := range c.Mail.ReservedLocalParts {
		if local == strings.ToLower(strings.TrimSpace(reserved)) {
			return true
		}
	}
	return false
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Server.HTTPAddr) == "" {
		return errors.New("server.httpAddr is required")
	}
	if strings.TrimSpace(c.SMTP.Addr) == "" {
		return errors.New("smtp.addr is required")
	}
	if len(c.Mail.Domains) == 0 {
		return errors.New("mail.domains must include at least one domain")
	}
	for _, domain := range c.Mail.Domains {
		if NormalizeDomain(domain) == "" {
			return errors.New("mail.domains cannot contain empty domains")
		}
	}
	if _, err := time.ParseDuration(c.Mail.Retention); err != nil {
		return fmt.Errorf("mail.retention: %w", err)
	}
	if c.Access.Mode != AccessPublic && c.Access.Mode != AccessPrivate {
		return fmt.Errorf("access.mode must be %q or %q", AccessPublic, AccessPrivate)
	}
	if strings.TrimSpace(c.Admin.Username) == "" {
		return errors.New("admin.username is required")
	}
	if c.API.Enabled && strings.TrimSpace(c.API.TokenHash) == "" {
		return errors.New("api.tokenHash is required when api.enabled is true")
	}
	if c.OpenAI.Enabled {
		switch strings.TrimSpace(c.OpenAI.APIMode) {
		case "", OpenAIAPIAuto, OpenAIAPIResponses, OpenAIAPIChatCompletions:
		default:
			return fmt.Errorf("openai.apiMode must be %q, %q, or %q", OpenAIAPIAuto, OpenAIAPIResponses, OpenAIAPIChatCompletions)
		}
		if strings.TrimSpace(c.OpenAI.BaseURL) == "" {
			return errors.New("openai.baseURL is required when openai.enabled is true")
		}
		if strings.TrimSpace(c.OpenAI.Model) == "" {
			return errors.New("openai.model is required when openai.enabled is true")
		}
		if _, err := c.OpenAITimeout(); err != nil {
			return fmt.Errorf("openai.timeout: %w", err)
		}
	}
	return nil
}

func (c Config) OpenAITimeout() (time.Duration, error) {
	timeout := strings.TrimSpace(c.OpenAI.Timeout)
	if timeout == "" {
		return 15 * time.Second, nil
	}
	duration, err := time.ParseDuration(timeout)
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, errors.New("must be positive")
	}
	return duration, nil
}

func NormalizeDomain(domain string) string {
	domain = strings.TrimSpace(strings.ToLower(domain))
	return strings.TrimSuffix(domain, ".")
}

func applyDefaults(cfg *Config) {
	defaults := Default()
	if strings.TrimSpace(cfg.Server.HTTPAddr) == "" {
		cfg.Server.HTTPAddr = defaults.Server.HTTPAddr
	}
	if strings.TrimSpace(cfg.SMTP.Addr) == "" {
		cfg.SMTP.Addr = defaults.SMTP.Addr
	}
	if len(cfg.Mail.Domains) == 0 {
		cfg.Mail.Domains = defaults.Mail.Domains
	}
	for i := range cfg.Mail.Domains {
		cfg.Mail.Domains[i] = NormalizeDomain(cfg.Mail.Domains[i])
	}
	if strings.TrimSpace(cfg.Mail.Retention) == "" {
		cfg.Mail.Retention = defaults.Mail.Retention
	}
	if len(cfg.Mail.ReservedLocalParts) == 0 {
		cfg.Mail.ReservedLocalParts = defaults.Mail.ReservedLocalParts
	}
	if strings.TrimSpace(cfg.Access.Mode) == "" {
		cfg.Access.Mode = AccessPublic
	}
	if strings.TrimSpace(cfg.Admin.Username) == "" {
		cfg.Admin.Username = defaults.Admin.Username
	}
	if strings.TrimSpace(cfg.OpenAI.BaseURL) == "" {
		cfg.OpenAI.BaseURL = defaults.OpenAI.BaseURL
	}
	if strings.TrimSpace(cfg.OpenAI.Model) == "" {
		cfg.OpenAI.Model = defaults.OpenAI.Model
	}
	if strings.TrimSpace(cfg.OpenAI.Timeout) == "" {
		cfg.OpenAI.Timeout = defaults.OpenAI.Timeout
	}
	if strings.TrimSpace(cfg.OpenAI.APIMode) == "" {
		cfg.OpenAI.APIMode = defaults.OpenAI.APIMode
	}
	if cfg.Webhooks == nil {
		cfg.Webhooks = []WebhookRule{}
	}
	if cfg.CodeProjects == nil {
		cfg.CodeProjects = []CodeProject{}
	}
}

func clone(cfg Config) Config {
	data, _ := json.Marshal(cfg)
	var copied Config
	_ = json.Unmarshal(data, &copied)
	return copied
}
