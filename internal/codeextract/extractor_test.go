package codeextract

import (
	"testing"
	"time"

	"github.com/PikachuCN/LeMail/internal/config"
	"github.com/PikachuCN/LeMail/internal/mailstore"
)

func TestExtractMatchesOpenAIH1CodeFromHTML(t *testing.T) {
	msg := mailstore.Message{
		ID:   "msg-openai",
		From: "OpenAI <noreply@tm.openai.com>",
		To:   []string{"amy904@localhost"},
		HTML: `<p>
            We noticed a suspicious log-in on your account. If that was you=
, enter this code:
        </p>
        <h1>892832</h1>
        <p>`,
		ReceivedAt: time.Unix(10, 0),
	}
	matches := ExtractMatches([]config.CodeProject{{
		ID:          "cp-chatgpt",
		Name:        "ChatGPT Signup",
		Enabled:     true,
		FromPattern: `(?i)openai\.com`,
		CodePattern: `(?is)<h1[^>]*>\s*(\d{6})\s*</h1>`,
		Source:      "html",
	}}, msg, nil)
	if len(matches) != 1 {
		t.Fatalf("expected one match, got %#v", matches)
	}
	if matches[0].Code != "892832" || matches[0].Mailbox != "amy904@localhost" || matches[0].ProjectName != "ChatGPT Signup" {
		t.Fatalf("unexpected match: %#v", matches[0])
	}
}

func TestExtractMatchesFromStrippedHTMLText(t *testing.T) {
	msg := mailstore.Message{
		ID:         "msg-html-text",
		From:       "service@example.com",
		To:         []string{"user@localhost"},
		HTML:       `<div><p>If that was you, enter this code:</p><h1>892832</h1></div>`,
		ReceivedAt: time.Unix(10, 0),
	}
	matches := ExtractMatches([]config.CodeProject{{
		ID:          "cp-generic",
		Name:        "Generic",
		Enabled:     true,
		CodePattern: `<h1>\s*(\d{6})\s*</h1>|enter this code:\s*(\d{6})`,
		Source:      "html",
	}}, msg, nil)
	if len(matches) != 1 || matches[0].Code != "892832" {
		t.Fatalf("expected code from stripped html text, got %#v", matches)
	}
}

func TestStoreCleansExpiredMatches(t *testing.T) {
	store := NewStore()
	store.AddMany([]Match{
		{ProjectID: "cp", MessageID: "old", Mailbox: "user@localhost", Code: "111111", ReceivedAt: time.Unix(10, 0)},
		{ProjectID: "cp", MessageID: "new", Mailbox: "user@localhost", Code: "222222", ReceivedAt: time.Unix(70, 0)},
	})
	removed := store.Cleanup(time.Unix(80, 0), time.Minute)
	if removed != 1 {
		t.Fatalf("expected one removed match, got %d", removed)
	}
	matches := store.ListMailbox("user@localhost")
	if len(matches) != 1 || matches[0].Code != "222222" {
		t.Fatalf("unexpected remaining matches: %#v", matches)
	}
}
