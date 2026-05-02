package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PikachuCN/LeMail/internal/config"
	"github.com/PikachuCN/LeMail/internal/mailstore"
)

func TestResponsesURLNormalizesOfficialOpenAIBaseURL(t *testing.T) {
	tests := map[string]string{
		"":                                              "https://api.openai.com/v1/responses",
		"https://api.openai.com":                        "https://api.openai.com/v1/responses",
		"https://api.openai.com/":                       "https://api.openai.com/v1/responses",
		"https://api.openai.com/v1":                     "https://api.openai.com/v1/responses",
		"https://api.openai.com/v1/":                    "https://api.openai.com/v1/responses",
		"https://api.openai.com/v1/responses":           "https://api.openai.com/v1/responses",
		"https://api.openai.com/v1/responses/":          "https://api.openai.com/v1/responses",
		"https://proxy.example.com/v1":                  "https://proxy.example.com/v1/responses",
		"https://proxy.example.com/v1/chat/completions": "https://proxy.example.com/v1/responses",
	}
	for input, want := range tests {
		if got := responsesURL(input); got != want {
			t.Fatalf("responsesURL(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestChatCompletionsURLNormalizesBaseURL(t *testing.T) {
	tests := map[string]string{
		"":                                    "https://api.openai.com/v1/chat/completions",
		"https://api.openai.com":              "https://api.openai.com/v1/chat/completions",
		"https://api.openai.com/v1":           "https://api.openai.com/v1/chat/completions",
		"https://api.openai.com/v1/responses": "https://api.openai.com/v1/chat/completions",
		"https://proxy.example.com/v1":        "https://proxy.example.com/v1/chat/completions",
		"https://proxy.example.com/v1/chat/completions": "https://proxy.example.com/v1/chat/completions",
		"https://proxy.example.com/openai/v1/responses": "https://proxy.example.com/openai/v1/chat/completions",
	}
	for input, want := range tests {
		if got := chatCompletionsURL(input); got != want {
			t.Fatalf("chatCompletionsURL(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestAutoModeFallsBackToChatCompletions(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("unexpected authorization: %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/responses":
			http.Error(w, "404 page not found", http.StatusNotFound)
		case "/chat/completions":
			if !strings.Contains(string(data), "654321") {
				t.Fatalf("expected prompt content in chat request: %s", string(data))
			}
			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{"content": `{"codes":[{"code":"654321","confidence":0.91,"reason":"chat fallback"}]}`}},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.OpenAI.Enabled = true
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.BaseURL = server.URL
	cfg.OpenAI.Model = "gpt-test"
	cfg.OpenAI.APIMode = config.OpenAIAPIAuto
	msg := mailstore.Message{
		ID:         "msg-ai-fallback",
		From:       "service@example.com",
		To:         []string{"user@localhost"},
		Subject:    "Code",
		Raw:        "edited code: 654321",
		ReceivedAt: time.Now(),
	}
	matches, err := NewOpenAIExtractor().ExtractCodes(context.Background(), cfg, config.CodeProject{
		ID:          "cp_test",
		Name:        "测试项目",
		CodePattern: `(\\d{6})`,
		Source:      "raw",
	}, msg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(paths, ",") != "/responses,/chat/completions" {
		t.Fatalf("unexpected request paths: %#v", paths)
	}
	if len(matches) != 1 || matches[0].Code != "654321" {
		t.Fatalf("unexpected matches: %#v", matches)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatal(err)
	}
}
