package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/PikachuCN/LeMail/internal/auth"
	"github.com/PikachuCN/LeMail/internal/codeextract"
	"github.com/PikachuCN/LeMail/internal/config"
	"github.com/PikachuCN/LeMail/internal/mailstore"
	"github.com/PikachuCN/LeMail/internal/realtime"
)

func TestMailboxWebSocketReceivesPublishedMail(t *testing.T) {
	manager := websocketTestManager(t, config.Default())
	store := mailstore.New()
	hub := realtime.NewHub()
	codeStore := codeextract.NewStore()
	api := NewWithCodes(manager, store, hub, auth.NewSessionManager(), codeStore, nil)
	server := httptest.NewServer(api.Routes())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/mailbox?address=user@localhost"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	expected := mailstore.Message{ID: "msg-ws", To: []string{"user@localhost"}, Subject: "Realtime", ReceivedAt: time.Now()}
	codeStore.AddMany([]codeextract.Match{{
		ProjectID:   "cp-chatgpt",
		ProjectName: "ChatGPT Signup",
		Mailbox:     "user@localhost",
		Code:        "892832",
		MessageID:   expected.ID,
		ReceivedAt:  expected.ReceivedAt,
	}})
	hub.Publish("user@localhost", expected)
	var payload struct {
		Type    string              `json:"type"`
		Message mailstore.Message   `json:"message"`
		Codes   []codeextract.Match `json:"codes"`
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Type != "mail" || payload.Message.ID != expected.ID {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if len(payload.Codes) != 1 || payload.Codes[0].Code != "892832" {
		t.Fatalf("expected websocket payload to include current codes, got %#v", payload.Codes)
	}
}

func TestPrivateMailboxWebSocketReceivesPublishedMailWithAccessCookie(t *testing.T) {
	cfg := config.Default()
	cfg.Access.Mode = config.AccessPrivate
	hash, err := auth.HashPassword("visitor-secret")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Access.PasswordHash = hash
	manager := websocketTestManager(t, cfg)
	store := mailstore.New()
	hub := realtime.NewHub()
	api := New(manager, store, hub, auth.NewSessionManager(), nil)
	server := httptest.NewServer(api.Routes())
	defer server.Close()

	resp, err := server.Client().Post(server.URL+"/api/access/login", "application/json", strings.NewReader(`{"password":"visitor-secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status=%d", resp.StatusCode)
	}
	if len(resp.Cookies()) == 0 {
		t.Fatal("expected access cookie")
	}

	header := http.Header{}
	header.Set("Cookie", resp.Cookies()[0].String())
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/mailbox?address=user@localhost"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	expected := mailstore.Message{ID: "msg-private-ws", To: []string{"user@localhost"}, Subject: "Private realtime", ReceivedAt: time.Now()}
	hub.Publish("user@localhost", expected)
	var payload struct {
		Type    string            `json:"type"`
		Message mailstore.Message `json:"message"`
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Type != "mail" || payload.Message.ID != expected.ID {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func websocketTestManager(t *testing.T, cfg config.Config) *config.Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := config.LoadManager(path)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}
