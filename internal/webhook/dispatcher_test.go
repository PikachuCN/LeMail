package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"log/slog"

	"github.com/PikachuCN/LeMail/internal/config"
	"github.com/PikachuCN/LeMail/internal/mailstore"
)

func TestDispatcherExtractsCodeAndPostsPayload(t *testing.T) {
	payloads := make(chan Payload, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var payload Payload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		payloads <- payload
	}))
	defer server.Close()

	manager := testManager(t)
	cfg := manager.Get()
	cfg.Webhooks = []config.WebhookRule{{
		ID:          "rule-1",
		Name:        "Codes",
		Enabled:     true,
		URL:         server.URL,
		Domains:     []string{"localhost"},
		CodePattern: `code is (\d{6})`,
		Source:      "text",
	}}
	if err := manager.Replace(cfg); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewDispatcher(manager, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	dispatcher.HandleMessage(mailstore.Message{
		ID:         "msg-1",
		From:       "sender@example.com",
		To:         []string{"user@localhost"},
		Subject:    "Login",
		Text:       "Your code is 123456",
		ReceivedAt: time.Unix(10, 0),
	})

	select {
	case payload := <-payloads:
		if payload.Code != "123456" || payload.RuleID != "rule-1" || payload.Mailbox != "user@localhost" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook")
	}
}

func TestValidateRuleRejectsUnsafeURL(t *testing.T) {
	rule := config.WebhookRule{Name: "bad", URL: "file:///tmp/out", CodePattern: `\d+`, Source: "text"}
	if err := ValidateRule(rule); err == nil {
		t.Fatal("expected invalid URL error")
	}
}

func testManager(t *testing.T) *config.Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	data, _ := json.Marshal(config.Default())
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := config.LoadManager(path)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}
