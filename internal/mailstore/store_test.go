package mailstore

import (
	"strings"
	"testing"
	"time"
)

func TestNewMessageParsesTextAndHTML(t *testing.T) {
	raw := strings.Join([]string{
		"From: Sender <sender@example.com>",
		"Subject: =?UTF-8?B?VGVzdCDpgq7ku7Y=?=",
		"Content-Type: multipart/alternative; boundary=demo",
		"",
		"--demo",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Your code is 123456",
		"--demo",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>Your code is <b>123456</b></p>",
		"--demo--",
	}, "\r\n")
	msg := NewMessage([]byte(raw), "bounce@example.com", []string{"User@Localhost"}, time.Unix(10, 0))
	if msg.From != "Sender <sender@example.com>" {
		t.Fatalf("unexpected from: %q", msg.From)
	}
	if msg.Subject != "Test 邮件" {
		t.Fatalf("unexpected subject: %q", msg.Subject)
	}
	if !strings.Contains(msg.Text, "123456") || !strings.Contains(msg.HTML, "<b>123456</b>") {
		t.Fatalf("message bodies were not parsed: text=%q html=%q", msg.Text, msg.HTML)
	}
	if got := msg.To[0]; got != "user@localhost" {
		t.Fatalf("recipient not normalized: %q", got)
	}
}

func TestStoreListsAndCleansByRetention(t *testing.T) {
	store := New()
	old := Message{ID: "old", To: []string{"a@localhost"}, ReceivedAt: time.Now().Add(-2 * time.Hour)}
	fresh := Message{ID: "fresh", To: []string{"a@localhost"}, ReceivedAt: time.Now()}
	store.Add(old)
	store.Add(fresh)
	if len(store.ListMailbox("a@localhost")) != 2 {
		t.Fatal("expected two messages before cleanup")
	}
	removed := store.Cleanup(time.Now(), time.Hour)
	if removed != 1 {
		t.Fatalf("expected one removed message, got %d", removed)
	}
	items := store.ListMailbox("a@localhost")
	if len(items) != 1 || items[0].ID != "fresh" {
		t.Fatalf("unexpected messages after cleanup: %#v", items)
	}
}
