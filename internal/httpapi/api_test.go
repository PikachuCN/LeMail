package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/PikachuCN/LeMail/internal/auth"
	"github.com/PikachuCN/LeMail/internal/codeextract"
	"github.com/PikachuCN/LeMail/internal/config"
	"github.com/PikachuCN/LeMail/internal/mailstore"
	"github.com/PikachuCN/LeMail/internal/realtime"
	"github.com/PikachuCN/LeMail/internal/smtpdebug"
)

func TestAdminSetupLoginAndMessages(t *testing.T) {
	api, _ := newTestAPI(t, config.Default())
	body := bytes.NewBufferString(`{"username":"root","password":"super-secret"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/setup", body)
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status=%d body=%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != adminCookie {
		t.Fatalf("expected admin cookie, got %#v", cookies)
	}
	var setupPayload struct {
		ConfigPath string `json:"configPath"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&setupPayload); err != nil {
		t.Fatal(err)
	}
	if setupPayload.ConfigPath == "" {
		t.Fatal("expected setup response to include configPath")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/admin/messages", nil)
	req.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("messages status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminSetupCreatesConfigFileWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	manager, err := config.LoadManager(path)
	if err != nil {
		t.Fatal(err)
	}
	api := New(manager, mailstore.New(), realtime.NewHub(), auth.NewSessionManager(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/setup", bytes.NewBufferString(`{"username":"root","password":"super-secret"}`))
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file to be created at %s: %v", path, err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.AddCookie(rec.Result().Cookies()[0])
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("config status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		ConfigPath string `json:"configPath"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.ConfigPath != path {
		t.Fatalf("expected configPath %q, got %q", path, payload.ConfigPath)
	}
}

func TestAdminDebugSMTPEventsAndDNS(t *testing.T) {
	api, _ := newTestAPI(t, config.Default())
	req := httptest.NewRequest(http.MethodPost, "/api/admin/setup", bytes.NewBufferString(`{"username":"root","password":"super-secret"}`))
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status=%d body=%s", rec.Code, rec.Body.String())
	}
	cookie := rec.Result().Cookies()[0]
	api.debug.Add(smtpdebug.Event{Type: smtpdebug.EventRcptReject, To: "user@example.org", Error: "domain is not accepted"})

	req = httptest.NewRequest(http.MethodGet, "/api/admin/debug/smtp/events", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("debug events status=%d body=%s", rec.Code, rec.Body.String())
	}
	var eventsPayload struct {
		Events []smtpdebug.Event `json:"events"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&eventsPayload); err != nil {
		t.Fatal(err)
	}
	if len(eventsPayload.Events) != 1 || eventsPayload.Events[0].Type != smtpdebug.EventRcptReject {
		t.Fatalf("unexpected debug events: %#v", eventsPayload.Events)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/admin/debug/dns", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dns status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"smtpAddr"`) {
		t.Fatalf("expected dns report, got %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/admin/debug/smtp/events", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear events status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(api.debug.List()) != 0 {
		t.Fatal("expected debug events to be cleared")
	}
}

func TestPrivateAccessProtectsMailboxEndpoints(t *testing.T) {
	cfg := config.Default()
	cfg.Access.Mode = config.AccessPrivate
	hash, err := auth.HashPassword("visitor-secret")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Access.PasswordHash = hash
	api, _ := newTestAPI(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/mailbox/random", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/access/login", bytes.NewBufferString(`{"password":"visitor-secret"}`))
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
	}
	cookie := rec.Result().Cookies()[0]
	req = httptest.NewRequest(http.MethodPost, "/api/mailbox/random", bytes.NewBufferString(`{}`))
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("random status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRandomMailboxUsesNaturalNameAndNumber(t *testing.T) {
	api, _ := newTestAPI(t, config.Default())
	req := httptest.NewRequest(http.MethodPost, "/api/mailbox/random", bytes.NewBufferString(`{"domain":"localhost"}`))
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("random status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		LocalPart string `json:"localPart"`
		Address   string `json:"address"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(payload.LocalPart, "mail") {
		t.Fatalf("local part should not use mail prefix: %q", payload.LocalPart)
	}
	if !regexp.MustCompile(`^[a-z]+[0-9]{3,4}$`).MatchString(payload.LocalPart) {
		t.Fatalf("local part should look like a real name plus digits: %q", payload.LocalPart)
	}
	if payload.Address != payload.LocalPart+"@localhost" {
		t.Fatalf("unexpected address: %q", payload.Address)
	}
}

func TestCodeProjectExtractsMailboxCodes(t *testing.T) {
	api, _ := newTestAPI(t, config.Default())
	req := httptest.NewRequest(http.MethodPost, "/api/admin/setup", bytes.NewBufferString(`{"username":"root","password":"super-secret"}`))
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status=%d body=%s", rec.Code, rec.Body.String())
	}
	adminCookie := rec.Result().Cookies()[0]

	api.store.Add(mailstore.Message{
		ID:         "msg-code",
		From:       "OpenAI <noreply@tm.openai.com>",
		To:         []string{"user@localhost"},
		Subject:    "Suspicious log-in",
		HTML:       `<p>enter this code:</p><h1>892832</h1><p>`,
		ReceivedAt: time.Now(),
	})

	body := bytes.NewBufferString(`{
		"name":"ChatGPT Signup",
		"enabled":true,
		"fromPattern":"(?i)openai\\.com",
		"codePattern":"(?is)<h1[^>]*>\\s*(\\d{6})\\s*</h1>",
		"source":"html"
	}`)
	req = httptest.NewRequest(http.MethodPost, "/api/admin/code-projects", body)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/mailbox/user@localhost/codes", nil)
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("codes status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Codes []struct {
			Code        string `json:"code"`
			ProjectName string `json:"projectName"`
		} `json:"codes"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Codes) != 1 || payload.Codes[0].Code != "892832" || payload.Codes[0].ProjectName != "ChatGPT Signup" {
		t.Fatalf("unexpected codes: %#v", payload.Codes)
	}
}

func TestAdminCanTestCodeProjectAgainstMessage(t *testing.T) {
	api, _ := newTestAPI(t, config.Default())
	req := httptest.NewRequest(http.MethodPost, "/api/admin/setup", bytes.NewBufferString(`{"username":"root","password":"super-secret"}`))
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status=%d body=%s", rec.Code, rec.Body.String())
	}
	adminCookie := rec.Result().Cookies()[0]

	api.store.Add(mailstore.Message{
		ID:         "msg-test-code",
		From:       "OpenAI <noreply@tm.openai.com>",
		To:         []string{"user@localhost"},
		Subject:    "Login code",
		HTML:       `<p>enter this code:</p><h1>892832</h1>`,
		ReceivedAt: time.Now(),
	})
	body := bytes.NewBufferString(`{
		"messageId":"msg-test-code",
		"project":{
			"name":"ChatGPT Signup",
			"enabled":false,
			"fromPattern":"(?i)openai\\.com",
			"codePattern":"(?is)<h1[^>]*>\\s*(\\d{6})\\s*</h1>",
			"source":"html"
		}
	}`)
	req = httptest.NewRequest(http.MethodPost, "/api/admin/code-projects/test", body)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("test status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Matches []struct {
			Code string `json:"code"`
		} `json:"matches"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Matches) != 1 || payload.Matches[0].Code != "892832" {
		t.Fatalf("unexpected test matches: %#v", payload.Matches)
	}
	if len(api.codes.ListMailbox("user@localhost")) != 0 {
		t.Fatal("test endpoint should not persist extraction results")
	}
}

func TestAdminCanGenerateRegexSuggestionsWithEditedSourceAndOpenAI(t *testing.T) {
	var authHeader string
	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected OpenAI path: %s", r.URL.Path)
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "654321") {
			t.Fatalf("expected edited source in OpenAI request: %s", string(data))
		}
		writeJSON(w, http.StatusOK, map[string]string{"output_text": `{"suggestions":[{"name":"Edited raw code","source":"raw","pattern":"edited code:\\s*(\\d{6})","sampleCode":"654321","confidence":0.98,"reason":"edited source"}]}`})
	}))
	defer openAIServer.Close()

	api, manager := newTestAPI(t, config.Default())
	req := httptest.NewRequest(http.MethodPost, "/api/admin/setup", bytes.NewBufferString(`{"username":"root","password":"super-secret"}`))
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status=%d body=%s", rec.Code, rec.Body.String())
	}
	adminCookie := rec.Result().Cookies()[0]

	cfg := manager.Get()
	cfg.OpenAI.Enabled = true
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.BaseURL = openAIServer.URL
	cfg.OpenAI.Model = "gpt-test"
	cfg.OpenAI.Timeout = "5s"
	if err := manager.Replace(cfg); err != nil {
		t.Fatal(err)
	}
	api.store.Add(mailstore.Message{
		ID:         "msg-ai-code",
		From:       "sender@example.com",
		To:         []string{"user@localhost"},
		Subject:    "Code",
		Raw:        "old raw",
		ReceivedAt: time.Now(),
	})

	body := bytes.NewBufferString(`{
		"messageId":"msg-ai-code",
		"suggestRegex":true,
		"project":{
			"name":"AI Test",
			"codePattern":"edited code: (\\d{6})",
			"source":"raw"
		},
		"message":{
			"from":"sender@example.com",
			"to":["user@localhost"],
			"subject":"Code",
			"text":"",
			"html":"",
			"raw":"edited code: 654321"
		}
	}`)
	req = httptest.NewRequest(http.MethodPost, "/api/admin/code-projects/test", body)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("test status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Matches []struct {
			Code string `json:"code"`
		} `json:"matches"`
		RegexSuggestions []struct {
			Pattern    string `json:"pattern"`
			Source     string `json:"source"`
			SampleCode string `json:"sampleCode"`
		} `json:"regexSuggestions"`
		AIError string `json:"aiError"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.AIError != "" {
		t.Fatalf("unexpected AI error: %s", payload.AIError)
	}
	if len(payload.Matches) != 1 || payload.Matches[0].Code != "654321" {
		t.Fatalf("unexpected regex matches from edited source: %#v", payload.Matches)
	}
	if len(payload.RegexSuggestions) != 1 ||
		payload.RegexSuggestions[0].Pattern != `edited code:\s*(\d{6})` ||
		payload.RegexSuggestions[0].Source != "raw" ||
		payload.RegexSuggestions[0].SampleCode != "654321" {
		t.Fatalf("unexpected regex suggestions: %#v", payload.RegexSuggestions)
	}
	if authHeader != "Bearer sk-test" {
		t.Fatalf("unexpected OpenAI authorization header: %q", authHeader)
	}
}

func TestAdminConfigDoesNotExposeOpenAIAPIKey(t *testing.T) {
	api, manager := newTestAPI(t, config.Default())
	req := httptest.NewRequest(http.MethodPost, "/api/admin/setup", bytes.NewBufferString(`{"username":"root","password":"super-secret"}`))
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status=%d body=%s", rec.Code, rec.Body.String())
	}
	adminCookie := rec.Result().Cookies()[0]

	cfg := manager.Get()
	cfg.OpenAI.Enabled = true
	cfg.OpenAI.APIKey = "sk-secret-value"
	if err := manager.Replace(cfg); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("config status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk-secret-value") {
		t.Fatalf("admin config response exposed OpenAI API key: %s", rec.Body.String())
	}
	var payload struct {
		OpenAI struct {
			APIKeySet bool `json:"apiKeySet"`
		} `json:"openai"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OpenAI.APIKeySet {
		t.Fatal("expected admin config response to report apiKeySet")
	}
}

func TestClientAPIReadsMailboxMessagesAndCodes(t *testing.T) {
	cfg := config.Default()
	hash, err := auth.HashPassword("lemail-api-token")
	if err != nil {
		t.Fatal(err)
	}
	cfg.API.Enabled = true
	cfg.API.TokenHash = hash
	api, _ := newTestAPI(t, cfg)

	msg := api.store.Add(mailstore.Message{
		ID:         "msg-client-api",
		From:       "service@example.com",
		To:         []string{"user@localhost"},
		Subject:    "Login code",
		Text:       "Your code is 123456",
		Raw:        "raw message with 123456",
		ReceivedAt: time.Now(),
	})
	api.codes.AddMany([]codeextract.Match{{
		ProjectID:   "cp_login",
		ProjectName: "登录验证码",
		Mailbox:     "user@localhost",
		Code:        "123456",
		Subject:     msg.Subject,
		From:        msg.From,
		ReceivedAt:  msg.ReceivedAt,
		MessageID:   msg.ID,
	}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mailboxes/user@localhost/messages", nil)
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized without token, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/mailboxes/random", bytes.NewBufferString(`{"domain":"localhost"}`))
	req.Header.Set("Authorization", "Bearer lemail-api-token")
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("random mailbox status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/mailboxes", bytes.NewBufferString(`{"localPart":"dev100","domain":"localhost"}`))
	req.Header.Set("Authorization", "Bearer lemail-api-token")
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("custom mailbox status=%d body=%s", rec.Code, rec.Body.String())
	}
	var mailboxPayload struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&mailboxPayload); err != nil {
		t.Fatal(err)
	}
	if mailboxPayload.Address != "dev100@localhost" {
		t.Fatalf("unexpected custom mailbox: %q", mailboxPayload.Address)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/mailboxes/user@localhost/messages", nil)
	req.Header.Set("Authorization", "Bearer lemail-api-token")
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("messages status=%d body=%s", rec.Code, rec.Body.String())
	}
	var messagesPayload struct {
		Messages []mailstore.Message `json:"messages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&messagesPayload); err != nil {
		t.Fatal(err)
	}
	if len(messagesPayload.Messages) != 1 || messagesPayload.Messages[0].ID != msg.ID {
		t.Fatalf("unexpected messages: %#v", messagesPayload.Messages)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/mailboxes/user@localhost/codes", nil)
	req.Header.Set("X-LeMail-API-Token", "lemail-api-token")
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("codes status=%d body=%s", rec.Code, rec.Body.String())
	}
	var codesPayload struct {
		Codes []codeextract.Match `json:"codes"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&codesPayload); err != nil {
		t.Fatal(err)
	}
	if len(codesPayload.Codes) != 1 || codesPayload.Codes[0].Code != "123456" {
		t.Fatalf("unexpected codes: %#v", codesPayload.Codes)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/messages/"+msg.ID, nil)
	req.Header.Set("Authorization", "Bearer lemail-api-token")
	rec = httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("message detail status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "raw message with 123456") {
		t.Fatalf("expected message detail to include raw content: %s", rec.Body.String())
	}
}

func newTestAPI(t *testing.T, cfg config.Config) (*API, *config.Manager) {
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
	api := New(manager, mailstore.New(), realtime.NewHub(), auth.NewSessionManager(), nil)
	return api, manager
}
