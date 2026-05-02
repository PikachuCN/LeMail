package smtpserver

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PikachuCN/LeMail/internal/codeextract"
	"github.com/PikachuCN/LeMail/internal/config"
	"github.com/PikachuCN/LeMail/internal/mailstore"
	"github.com/PikachuCN/LeMail/internal/realtime"
	"github.com/PikachuCN/LeMail/internal/smtpdebug"
)

func TestSessionStoresAndPublishesMail(t *testing.T) {
	manager := smtpTestManager(t)
	store := mailstore.New()
	hub := realtime.NewHub()
	srv := New(manager, store, hub, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	sub := hub.Subscribe("user@localhost")
	defer sub.Close()
	sess := &session{server: srv}
	if err := sess.Mail("sender@example.com", nil); err != nil {
		t.Fatal(err)
	}
	if err := sess.Rcpt("User@Localhost", nil); err != nil {
		t.Fatal(err)
	}
	raw := strings.Join([]string{
		"From: Sender <sender@example.com>",
		"Subject: Integration",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hello from SMTP",
	}, "\r\n")
	if err := sess.Data(strings.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	items := store.ListMailbox("user@localhost")
	if len(items) != 1 || items[0].Subject != "Integration" {
		t.Fatalf("unexpected stored mail: %#v", items)
	}
	select {
	case msg := <-sub.C:
		if msg.ID != items[0].ID {
			t.Fatalf("published message mismatch: %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for realtime publish")
	}
}

func TestSessionExtractsCodesBeforeRealtimePublish(t *testing.T) {
	manager := smtpTestManager(t)
	cfg := manager.Get()
	cfg.CodeProjects = []config.CodeProject{{
		ID:          "cp-openai",
		Name:        "OpenAI",
		Enabled:     true,
		FromPattern: `(?i)openai\.com`,
		CodePattern: `(?is)<h1[^>]*>\s*(\d{6})\s*</h1>`,
		Source:      "html",
	}}
	if err := manager.Replace(cfg); err != nil {
		t.Fatal(err)
	}
	store := mailstore.New()
	codeStore := codeextract.NewStore()
	hub := realtime.NewHub()
	processor := codeextract.NewProcessor(manager, codeStore, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := New(manager, store, hub, slog.New(slog.NewTextHandler(io.Discard, nil)), processor.HandleMessage)
	sub := hub.Subscribe("user@localhost")
	defer sub.Close()

	sess := &session{server: srv}
	if err := sess.Mail("noreply@tm.openai.com", nil); err != nil {
		t.Fatal(err)
	}
	if err := sess.Rcpt("user@localhost", nil); err != nil {
		t.Fatal(err)
	}
	raw := strings.Join([]string{
		"From: OpenAI <noreply@tm.openai.com>",
		"Subject: Login code",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>enter this code:</p><h1>892832</h1>",
	}, "\r\n")
	if err := sess.Data(strings.NewReader(raw)); err != nil {
		t.Fatal(err)
	}

	select {
	case <-sub.C:
		matches := codeStore.ListMailbox("user@localhost")
		if len(matches) != 1 || matches[0].Code != "892832" {
			t.Fatalf("expected code to be available when realtime publish is received, got %#v", matches)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for realtime publish")
	}
}

func TestSessionRejectsUnknownDomain(t *testing.T) {
	srv := New(smtpTestManager(t), mailstore.New(), realtime.NewHub(), slog.New(slog.NewTextHandler(os.Stdout, nil)), nil)
	sess := &session{server: srv}
	if err := sess.Rcpt("user@example.org", nil); err == nil {
		t.Fatal("expected domain rejection")
	}
}

func TestSessionRecordsDebugEvents(t *testing.T) {
	debugStore := smtpdebug.New(20)
	srv := NewWithDebug(smtpTestManager(t), mailstore.New(), realtime.NewHub(), slog.New(slog.NewTextHandler(io.Discard, nil)), nil, debugStore)
	sess := &session{server: srv, id: "test-session", remoteAddr: "192.0.2.10:12345", helo: "sender.example"}

	if err := sess.Mail("sender@example.com", nil); err != nil {
		t.Fatal(err)
	}
	if err := sess.Rcpt("user@example.org", nil); err == nil {
		t.Fatal("expected domain rejection")
	}
	if err := sess.Rcpt("user@localhost", nil); err != nil {
		t.Fatal(err)
	}
	if err := sess.Data(strings.NewReader("Subject: Debug\r\n\r\nHello")); err != nil {
		t.Fatal(err)
	}

	events := debugStore.List()
	var hasReject, hasAccept, hasStored bool
	for _, event := range events {
		switch event.Type {
		case smtpdebug.EventRcptReject:
			hasReject = event.To == "user@example.org" && event.Error != ""
		case smtpdebug.EventRcptAccept:
			hasAccept = event.To == "user@localhost"
		case smtpdebug.EventMailStored:
			hasStored = event.MessageID != "" && event.Size > 0
		}
	}
	if !hasReject || !hasAccept || !hasStored {
		t.Fatalf("missing debug events: %#v", events)
	}
}

func smtpTestManager(t *testing.T) *config.Manager {
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
