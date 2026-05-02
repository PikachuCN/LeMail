package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"math/big"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/PikachuCN/LeMail/internal/ai"
	"github.com/PikachuCN/LeMail/internal/auth"
	"github.com/PikachuCN/LeMail/internal/codeextract"
	"github.com/PikachuCN/LeMail/internal/config"
	"github.com/PikachuCN/LeMail/internal/mailstore"
	"github.com/PikachuCN/LeMail/internal/realtime"
	"github.com/PikachuCN/LeMail/internal/smtpdebug"
	"github.com/PikachuCN/LeMail/internal/webhook"
)

const (
	accessCookie = "fm_access"
	adminCookie  = "fm_admin"
	accessKind   = "access"
	adminKind    = "admin"
)

var localPartPattern = regexp.MustCompile(`^[a-z0-9._%+\-]{1,64}$`)

var randomNameSeeds = []string{
	"alex", "amy", "anna", "ben", "carl", "chloe", "david", "emma", "eric", "eva",
	"fiona", "frank", "grace", "henry", "iris", "jack", "jason", "kate", "leo", "lily",
	"lucas", "luna", "mason", "mia", "nina", "noah", "oliver", "oscar", "paul", "rose",
	"sara", "simon", "sophia", "tom", "victor", "wendy", "zoe",
}

type API struct {
	manager  *config.Manager
	store    *mailstore.Store
	codes    *codeextract.Store
	hub      *realtime.Hub
	sessions *auth.SessionManager
	static   fs.FS
	debug    *smtpdebug.Store
	upgrader websocket.Upgrader
}

func New(manager *config.Manager, store *mailstore.Store, hub *realtime.Hub, sessions *auth.SessionManager, static fs.FS) *API {
	return NewWithCodes(manager, store, hub, sessions, codeextract.NewStore(), static)
}

func NewWithCodes(manager *config.Manager, store *mailstore.Store, hub *realtime.Hub, sessions *auth.SessionManager, codes *codeextract.Store, static fs.FS) *API {
	return NewWithCodesAndDebug(manager, store, hub, sessions, codes, nil, static)
}

func NewWithCodesAndDebug(manager *config.Manager, store *mailstore.Store, hub *realtime.Hub, sessions *auth.SessionManager, codes *codeextract.Store, debug *smtpdebug.Store, static fs.FS) *API {
	if codes == nil {
		codes = codeextract.NewStore()
	}
	if debug == nil {
		debug = smtpdebug.New(smtpdebug.DefaultLimit)
	}
	return &API{
		manager:  manager,
		store:    store,
		codes:    codes,
		hub:      hub,
		sessions: sessions,
		static:   static,
		debug:    debug,
		upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/public/config", a.handlePublicConfig)
	mux.HandleFunc("POST /api/access/login", a.handleAccessLogin)
	mux.HandleFunc("POST /api/mailbox/random", a.handleRandomMailbox)
	mux.HandleFunc("GET /api/mailbox/", a.handleMailboxResource)
	mux.HandleFunc("GET /ws/mailbox", a.handleMailboxWebSocket)

	mux.HandleFunc("GET /api/v1/config", a.requireClientAPI(a.handleClientConfig))
	mux.HandleFunc("POST /api/v1/mailboxes/random", a.requireClientAPI(a.handleClientRandomMailbox))
	mux.HandleFunc("POST /api/v1/mailboxes", a.requireClientAPI(a.handleClientMailboxCreate))
	mux.HandleFunc("GET /api/v1/mailboxes/", a.requireClientAPI(a.handleClientMailboxResource))
	mux.HandleFunc("GET /api/v1/messages/", a.requireClientAPI(a.handleClientMessage))

	mux.HandleFunc("POST /api/admin/setup", a.handleAdminSetup)
	mux.HandleFunc("POST /api/admin/login", a.handleAdminLogin)
	mux.HandleFunc("GET /api/admin/messages", a.requireAdmin(a.handleAdminMessages))
	mux.HandleFunc("GET /api/admin/codes", a.requireAdmin(a.handleAdminCodes))
	mux.HandleFunc("GET /api/admin/config", a.requireAdmin(a.handleAdminConfig))
	mux.HandleFunc("PUT /api/admin/config", a.requireAdmin(a.handleAdminConfigUpdate))
	mux.HandleFunc("GET /api/admin/debug/smtp/events", a.requireAdmin(a.handleAdminDebugSMTPEvents))
	mux.HandleFunc("DELETE /api/admin/debug/smtp/events", a.requireAdmin(a.handleAdminDebugSMTPClear))
	mux.HandleFunc("GET /api/admin/debug/dns", a.requireAdmin(a.handleAdminDebugDNS))
	mux.HandleFunc("GET /api/admin/code-projects", a.requireAdmin(a.handleAdminCodeProjects))
	mux.HandleFunc("POST /api/admin/code-projects", a.requireAdmin(a.handleAdminCodeProjectCreate))
	mux.HandleFunc("POST /api/admin/code-projects/test", a.requireAdmin(a.handleAdminCodeProjectTest))
	mux.HandleFunc("PUT /api/admin/code-projects/", a.requireAdmin(a.handleAdminCodeProjectUpdate))
	mux.HandleFunc("DELETE /api/admin/code-projects/", a.requireAdmin(a.handleAdminCodeProjectDelete))
	mux.HandleFunc("GET /api/admin/webhooks", a.requireAdmin(a.handleAdminWebhooks))
	mux.HandleFunc("POST /api/admin/webhooks", a.requireAdmin(a.handleAdminWebhookCreate))
	mux.HandleFunc("PUT /api/admin/webhooks/", a.requireAdmin(a.handleAdminWebhookUpdate))
	mux.HandleFunc("DELETE /api/admin/webhooks/", a.requireAdmin(a.handleAdminWebhookDelete))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		a.serveFrontend(w, r)
	})
	return withCORS(mux)
}

func (a *API) serveFrontend(w http.ResponseWriter, r *http.Request) {
	if a.static == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><title>LeMail</title><h1>LeMail</h1><p>Frontend assets are not built yet.</p>"))
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "." || name == "" {
		name = "index.html"
	}
	if _, err := fs.Stat(a.static, name); err != nil {
		name = "index.html"
	}
	data, err := fs.ReadFile(a.static, name)
	if err != nil {
		writeError(w, http.StatusNotFound, "frontend asset not found")
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

func (a *API) handlePublicConfig(w http.ResponseWriter, r *http.Request) {
	cfg := a.manager.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"domains":            cfg.Mail.Domains,
		"accessMode":         cfg.Access.Mode,
		"requiresAccess":     cfg.Access.Mode == config.AccessPrivate,
		"adminSetupRequired": cfg.Admin.PasswordHash == "",
		"retention":          cfg.Mail.Retention,
	})
}

func (a *API) handleAccessLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&input)
	cfg := a.manager.Get()
	if cfg.Access.Mode == config.AccessPrivate && !auth.CheckPassword(cfg.Access.PasswordHash, input.Password) {
		writeError(w, http.StatusUnauthorized, "invalid access password")
		return
	}
	ttl := 24 * time.Hour
	token := a.sessions.Create(accessKind, "visitor", ttl)
	setSessionCookie(w, accessCookie, token, ttl)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) handleRandomMailbox(w http.ResponseWriter, r *http.Request) {
	if !a.requireAccess(w, r) {
		return
	}
	var input struct {
		Domain string `json:"domain"`
	}
	_ = json.NewDecoder(r.Body).Decode(&input)
	cfg := a.manager.Get()
	domain := config.NormalizeDomain(input.Domain)
	if domain == "" && len(cfg.Mail.Domains) > 0 {
		domain = cfg.Mail.Domains[0]
	}
	if !cfg.HasDomain(domain) {
		writeError(w, http.StatusBadRequest, "domain is not accepted")
		return
	}
	local := randomLocalPart()
	for cfg.IsReservedLocalPart(local) {
		local = randomLocalPart()
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"localPart": local,
		"domain":    domain,
		"address":   local + "@" + domain,
	})
}

func (a *API) handleMailboxResource(w http.ResponseWriter, r *http.Request) {
	if !a.requireAccess(w, r) {
		return
	}
	resource, ok := strings.CutPrefix(r.URL.Path, "/api/mailbox/")
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	var address string
	var response any
	switch {
	case strings.HasSuffix(resource, "/messages"):
		address = strings.TrimSuffix(resource, "/messages")
		response = map[string]any{"messages": nil}
	case strings.HasSuffix(resource, "/codes"):
		address = strings.TrimSuffix(resource, "/codes")
		response = map[string]any{"codes": nil}
	default:
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	address, _ = url.PathUnescape(address)
	if err := a.validateMailbox(address); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.HasSuffix(resource, "/messages") {
		response = map[string]any{"messages": a.store.ListMailbox(address)}
	} else {
		response = map[string]any{"codes": a.codes.ListMailbox(address)}
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *API) handleMailboxWebSocket(w http.ResponseWriter, r *http.Request) {
	if !a.requireAccess(w, r) {
		return
	}
	address := r.URL.Query().Get("address")
	if err := a.validateMailbox(address); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Subscribe before completing the upgrade so messages arriving right after
	// the client connects are not lost in the handshake window.
	sub := a.hub.Subscribe(address)
	defer sub.Close()
	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()
	for {
		select {
		case msg := <-sub.C:
			if err := conn.WriteJSON(map[string]any{
				"type":    "mail",
				"message": msg,
				"codes":   a.codes.ListMailbox(address),
			}); err != nil {
				return
			}
		case <-done:
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (a *API) handleClientConfig(w http.ResponseWriter, r *http.Request) {
	cfg := a.manager.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"version":    "v1",
		"domains":    cfg.Mail.Domains,
		"retention":  cfg.Mail.Retention,
		"accessMode": cfg.Access.Mode,
	})
}

func (a *API) handleClientRandomMailbox(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Domain string `json:"domain"`
	}
	_ = json.NewDecoder(r.Body).Decode(&input)
	cfg := a.manager.Get()
	domain := config.NormalizeDomain(input.Domain)
	if domain == "" && len(cfg.Mail.Domains) > 0 {
		domain = cfg.Mail.Domains[0]
	}
	if !cfg.HasDomain(domain) {
		writeError(w, http.StatusBadRequest, "domain is not accepted")
		return
	}
	local := randomLocalPart()
	for cfg.IsReservedLocalPart(local) {
		local = randomLocalPart()
	}
	writeMailboxAddress(w, http.StatusOK, local, domain)
}

func (a *API) handleClientMailboxCreate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Address   string `json:"address"`
		LocalPart string `json:"localPart"`
		Domain    string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	address := strings.ToLower(strings.TrimSpace(input.Address))
	if address == "" {
		local := strings.ToLower(strings.TrimSpace(input.LocalPart))
		domain := config.NormalizeDomain(input.Domain)
		address = local + "@" + domain
	}
	if err := a.validateMailbox(address); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	local, domain, _ := strings.Cut(address, "@")
	writeMailboxAddress(w, http.StatusOK, local, domain)
}

func (a *API) handleClientMailboxResource(w http.ResponseWriter, r *http.Request) {
	resource, ok := strings.CutPrefix(r.URL.Path, "/api/v1/mailboxes/")
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	var address string
	switch {
	case strings.HasSuffix(resource, "/messages"):
		address = strings.TrimSuffix(resource, "/messages")
	case strings.HasSuffix(resource, "/codes"):
		address = strings.TrimSuffix(resource, "/codes")
	default:
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	address, _ = url.PathUnescape(address)
	if err := a.validateMailbox(address); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.HasSuffix(resource, "/messages") {
		writeJSON(w, http.StatusOK, map[string]any{"messages": a.store.ListMailbox(address)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"codes": a.codes.ListMailbox(address)})
}

func (a *API) handleClientMessage(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/messages/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	msg, ok := a.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": msg})
}

func (a *API) handleAdminSetup(w http.ResponseWriter, r *http.Request) {
	cfg := a.manager.Get()
	if cfg.Admin.PasswordHash != "" {
		writeError(w, http.StatusConflict, "admin already configured")
		return
	}
	var input struct {
		Username       string `json:"username"`
		Password       string `json:"password"`
		AccessPassword string `json:"accessPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if strings.TrimSpace(input.Username) != "" {
		cfg.Admin.Username = strings.TrimSpace(input.Username)
	}
	if len(input.Password) < 8 {
		writeError(w, http.StatusBadRequest, "admin password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	cfg.Admin.PasswordHash = hash
	if input.AccessPassword != "" {
		accessHash, err := auth.HashPassword(input.AccessPassword)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to hash access password")
			return
		}
		cfg.Access.PasswordHash = accessHash
	}
	if err := a.saveConfig(cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ttl := 12 * time.Hour
	token := a.sessions.Create(adminKind, cfg.Admin.Username, ttl)
	setSessionCookie(w, adminCookie, token, ttl)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "configPath": a.manager.Path()})
}

func (a *API) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&input)
	cfg := a.manager.Get()
	if cfg.Admin.PasswordHash == "" {
		writeError(w, http.StatusConflict, "admin setup required")
		return
	}
	if input.Username != cfg.Admin.Username || !auth.CheckPassword(cfg.Admin.PasswordHash, input.Password) {
		writeError(w, http.StatusUnauthorized, "invalid admin credentials")
		return
	}
	ttl := 12 * time.Hour
	token := a.sessions.Create(adminKind, cfg.Admin.Username, ttl)
	setSessionCookie(w, adminCookie, token, ttl)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) handleAdminMessages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"messages": a.store.ListAll()})
}

func (a *API) handleAdminCodes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"codes": a.codes.ListAll()})
}

func (a *API) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, adminConfigResponseFrom(a.manager.Get(), a.manager.Path()))
}

func (a *API) handleAdminDebugSMTPEvents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"events": a.debug.List()})
}

func (a *API) handleAdminDebugSMTPClear(w http.ResponseWriter, r *http.Request) {
	a.debug.Clear()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) handleAdminDebugDNS(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, smtpdebug.CheckDNS(a.manager.Get()))
}

func (a *API) handleAdminConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var input adminConfigUpdate
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	current := a.manager.Get()
	updated := current
	updated.Server = input.Server
	updated.SMTP = input.SMTP
	updated.Mail = input.Mail
	updated.Access.Mode = input.Access.Mode
	if input.Access.Password != "" {
		hash, err := auth.HashPassword(input.Access.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to hash access password")
			return
		}
		updated.Access.PasswordHash = hash
	}
	if strings.TrimSpace(input.Admin.Username) != "" {
		updated.Admin.Username = strings.TrimSpace(input.Admin.Username)
	}
	if input.Admin.Password != "" {
		if len(input.Admin.Password) < 8 {
			writeError(w, http.StatusBadRequest, "admin password must be at least 8 characters")
			return
		}
		hash, err := auth.HashPassword(input.Admin.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to hash admin password")
			return
		}
		updated.Admin.PasswordHash = hash
	}
	updated.API.Enabled = input.API.Enabled
	if input.API.ClearToken {
		updated.API.TokenHash = ""
	} else if strings.TrimSpace(input.API.Token) != "" {
		if len(input.API.Token) < 12 {
			writeError(w, http.StatusBadRequest, "api token must be at least 12 characters")
			return
		}
		hash, err := auth.HashPassword(input.API.Token)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to hash api token")
			return
		}
		updated.API.TokenHash = hash
	}
	updated.OpenAI.Enabled = input.OpenAI.Enabled
	updated.OpenAI.BaseURL = input.OpenAI.BaseURL
	updated.OpenAI.Model = input.OpenAI.Model
	updated.OpenAI.Timeout = input.OpenAI.Timeout
	updated.OpenAI.APIMode = input.OpenAI.APIMode
	if input.OpenAI.ClearAPIKey {
		updated.OpenAI.APIKey = ""
	} else if strings.TrimSpace(input.OpenAI.APIKey) != "" {
		updated.OpenAI.APIKey = strings.TrimSpace(input.OpenAI.APIKey)
	}
	updated.Webhooks = input.Webhooks
	updated.CodeProjects = input.CodeProjects
	if err := a.saveConfig(updated); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a.rebuildCodeMatches()
	writeJSON(w, http.StatusOK, adminConfigResponseFrom(a.manager.Get(), a.manager.Path()))
}

func (a *API) handleAdminCodeProjects(w http.ResponseWriter, r *http.Request) {
	projects := a.manager.Get().CodeProjects
	if projects == nil {
		projects = []config.CodeProject{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"codeProjects": projects})
}

func (a *API) handleAdminCodeProjectCreate(w http.ResponseWriter, r *http.Request) {
	var project config.CodeProject
	if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if project.ID == "" {
		project.ID = "cp_" + randomHex(6)
	}
	if project.Source == "" {
		project.Source = "all"
	}
	if err := codeextract.ValidateProject(project); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg := a.manager.Get()
	for _, existing := range cfg.CodeProjects {
		if existing.ID == project.ID {
			writeError(w, http.StatusConflict, "code project id already exists")
			return
		}
	}
	cfg.CodeProjects = append(cfg.CodeProjects, project)
	if err := a.saveConfig(cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a.rebuildCodeMatches()
	writeJSON(w, http.StatusCreated, project)
}

func (a *API) handleAdminCodeProjectTest(w http.ResponseWriter, r *http.Request) {
	var input struct {
		MessageID    string             `json:"messageId"`
		Project      config.CodeProject `json:"project"`
		Message      *messageOverride   `json:"message,omitempty"`
		UseAI        bool               `json:"useAI"`
		SuggestRegex bool               `json:"suggestRegex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	msg, ok := a.store.Get(input.MessageID)
	if !ok {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	if input.Message != nil {
		msg = input.Message.Apply(msg)
	}
	project := input.Project
	if project.ID == "" {
		project.ID = "cp_test"
	}
	if project.Name == "" {
		project.Name = "测试项目"
	}
	if project.Source == "" {
		project.Source = "all"
	}
	project.Enabled = true
	if err := codeextract.ValidateProject(project); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload := map[string]any{"matches": codeextract.ExtractMatches([]config.CodeProject{project}, msg, nil)}
	if input.UseAI || input.SuggestRegex {
		suggestions, err := ai.NewOpenAIExtractor().SuggestRegex(r.Context(), a.manager.Get(), project, msg)
		if err != nil {
			payload["aiError"] = err.Error()
			payload["regexSuggestions"] = []ai.RegexSuggestion{}
		} else {
			payload["regexSuggestions"] = suggestions
		}
	}
	writeJSON(w, http.StatusOK, payload)
}

func (a *API) handleAdminCodeProjectUpdate(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/code-projects/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	var project config.CodeProject
	if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	project.ID = id
	if project.Source == "" {
		project.Source = "all"
	}
	if err := codeextract.ValidateProject(project); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg := a.manager.Get()
	for i := range cfg.CodeProjects {
		if cfg.CodeProjects[i].ID == id {
			cfg.CodeProjects[i] = project
			if err := a.saveConfig(cfg); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			a.rebuildCodeMatches()
			writeJSON(w, http.StatusOK, project)
			return
		}
	}
	writeError(w, http.StatusNotFound, "code project not found")
}

func (a *API) handleAdminCodeProjectDelete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/code-projects/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	cfg := a.manager.Get()
	kept := cfg.CodeProjects[:0]
	found := false
	for _, project := range cfg.CodeProjects {
		if project.ID == id {
			found = true
			continue
		}
		kept = append(kept, project)
	}
	if !found {
		writeError(w, http.StatusNotFound, "code project not found")
		return
	}
	cfg.CodeProjects = kept
	if err := a.saveConfig(cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a.rebuildCodeMatches()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) handleAdminWebhooks(w http.ResponseWriter, r *http.Request) {
	webhooks := a.manager.Get().Webhooks
	if webhooks == nil {
		webhooks = []config.WebhookRule{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhooks": webhooks})
}

func (a *API) handleAdminWebhookCreate(w http.ResponseWriter, r *http.Request) {
	var rule config.WebhookRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if rule.ID == "" {
		rule.ID = "wh_" + randomHex(6)
	}
	if rule.Source == "" {
		rule.Source = "all"
	}
	if err := webhook.ValidateRule(rule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg := a.manager.Get()
	for _, existing := range cfg.Webhooks {
		if existing.ID == rule.ID {
			writeError(w, http.StatusConflict, "webhook id already exists")
			return
		}
	}
	cfg.Webhooks = append(cfg.Webhooks, rule)
	if err := a.saveConfig(cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (a *API) handleAdminWebhookUpdate(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/webhooks/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	var rule config.WebhookRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	rule.ID = id
	if rule.Source == "" {
		rule.Source = "all"
	}
	if err := webhook.ValidateRule(rule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg := a.manager.Get()
	for i := range cfg.Webhooks {
		if cfg.Webhooks[i].ID == id {
			cfg.Webhooks[i] = rule
			if err := a.saveConfig(cfg); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, rule)
			return
		}
	}
	writeError(w, http.StatusNotFound, "webhook not found")
}

func (a *API) handleAdminWebhookDelete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/webhooks/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	cfg := a.manager.Get()
	kept := cfg.Webhooks[:0]
	found := false
	for _, rule := range cfg.Webhooks {
		if rule.ID == id {
			found = true
			continue
		}
		kept = append(kept, rule)
	}
	if !found {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}
	cfg.Webhooks = kept
	if err := a.saveConfig(cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) saveConfig(cfg config.Config) error {
	if cfg.Access.Mode == config.AccessPrivate && cfg.Access.PasswordHash == "" {
		return errors.New("access password is required in private mode")
	}
	if cfg.Admin.PasswordHash == "" {
		return errors.New("admin password is required")
	}
	if cfg.API.Enabled && cfg.API.TokenHash == "" {
		return errors.New("api token is required when api is enabled")
	}
	for i := range cfg.Webhooks {
		if cfg.Webhooks[i].ID == "" {
			cfg.Webhooks[i].ID = "wh_" + randomHex(6)
		}
		if cfg.Webhooks[i].Source == "" {
			cfg.Webhooks[i].Source = "all"
		}
		if err := webhook.ValidateRule(cfg.Webhooks[i]); err != nil {
			return err
		}
	}
	for i := range cfg.CodeProjects {
		if cfg.CodeProjects[i].ID == "" {
			cfg.CodeProjects[i].ID = "cp_" + randomHex(6)
		}
		if cfg.CodeProjects[i].Source == "" {
			cfg.CodeProjects[i].Source = "all"
		}
		if err := codeextract.ValidateProject(cfg.CodeProjects[i]); err != nil {
			return err
		}
	}
	return a.manager.Save(cfg)
}

func (a *API) rebuildCodeMatches() {
	cfg := a.manager.Get()
	matches := []codeextract.Match{}
	for _, msg := range a.store.ListAll() {
		matches = append(matches, codeextract.ExtractMatches(cfg.CodeProjects, msg, nil)...)
	}
	a.codes.Replace(matches)
}

func (a *API) requireAccess(w http.ResponseWriter, r *http.Request) bool {
	cfg := a.manager.Get()
	if cfg.Access.Mode == config.AccessPublic {
		return true
	}
	if a.hasSession(r, accessCookie, accessKind) || a.hasSession(r, adminCookie, adminKind) {
		return true
	}
	writeError(w, http.StatusUnauthorized, "access password required")
	return false
}

func (a *API) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.hasSession(r, adminCookie, adminKind) {
			writeError(w, http.StatusUnauthorized, "admin login required")
			return
		}
		next(w, r)
	}
}

func (a *API) requireClientAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := a.manager.Get()
		if !cfg.API.Enabled {
			writeError(w, http.StatusForbidden, "client api is disabled")
			return
		}
		token := clientAPIToken(r)
		if !auth.CheckPassword(cfg.API.TokenHash, token) {
			writeError(w, http.StatusUnauthorized, "invalid api token")
			return
		}
		next(w, r)
	}
}

func (a *API) hasSession(r *http.Request, cookieName string, kind string) bool {
	if cookie, err := r.Cookie(cookieName); err == nil && a.sessions.Validate(cookie.Value, kind) {
		return true
	}
	bearer := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
	if bearer != "" && a.sessions.Validate(bearer, kind) {
		return true
	}
	queryToken := r.URL.Query().Get(cookieName)
	return queryToken != "" && a.sessions.Validate(queryToken, kind)
}

func clientAPIToken(r *http.Request) string {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(r.Header.Get("X-LeMail-API-Token"))
}

func (a *API) validateMailbox(address string) error {
	address = strings.ToLower(strings.TrimSpace(address))
	local, domain, ok := strings.Cut(address, "@")
	if !ok || local == "" || domain == "" {
		return errors.New("invalid mailbox address")
	}
	if !localPartPattern.MatchString(local) {
		return errors.New("invalid mailbox local part")
	}
	cfg := a.manager.Get()
	if !cfg.HasDomain(domain) {
		return errors.New("domain is not accepted")
	}
	if cfg.IsReservedLocalPart(local) {
		return errors.New("local part is reserved")
	}
	return nil
}

type messageOverride struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	HTML    string   `json:"html"`
	Raw     string   `json:"raw"`
}

func (m messageOverride) Apply(msg mailstore.Message) mailstore.Message {
	msg.From = m.From
	if len(m.To) > 0 {
		msg.To = m.To
	}
	msg.Subject = m.Subject
	msg.Text = m.Text
	msg.HTML = m.HTML
	msg.Raw = m.Raw
	return msg
}

type adminConfigUpdate struct {
	Server       config.ServerConfig  `json:"server"`
	SMTP         config.SMTPConfig    `json:"smtp"`
	Mail         config.MailConfig    `json:"mail"`
	Access       accessConfigUpdate   `json:"access"`
	Admin        adminAccountUpdate   `json:"admin"`
	API          apiConfigUpdate      `json:"api"`
	OpenAI       openAIConfigUpdate   `json:"openai"`
	Webhooks     []config.WebhookRule `json:"webhooks"`
	CodeProjects []config.CodeProject `json:"codeProjects"`
}

type accessConfigUpdate struct {
	Mode     string `json:"mode"`
	Password string `json:"password,omitempty"`
}

type adminAccountUpdate struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
}

type apiConfigUpdate struct {
	Enabled    bool   `json:"enabled"`
	Token      string `json:"token,omitempty"`
	ClearToken bool   `json:"clearToken,omitempty"`
}

type openAIConfigUpdate struct {
	Enabled     bool   `json:"enabled"`
	APIKey      string `json:"apiKey,omitempty"`
	ClearAPIKey bool   `json:"clearApiKey,omitempty"`
	BaseURL     string `json:"baseURL"`
	Model       string `json:"model"`
	Timeout     string `json:"timeout"`
	APIMode     string `json:"apiMode"`
}

type adminConfigResponse struct {
	ConfigPath   string               `json:"configPath"`
	Server       config.ServerConfig  `json:"server"`
	SMTP         config.SMTPConfig    `json:"smtp"`
	Mail         config.MailConfig    `json:"mail"`
	Access       accessConfigResponse `json:"access"`
	Admin        adminAccountResponse `json:"admin"`
	API          apiConfigResponse    `json:"api"`
	OpenAI       openAIConfigResponse `json:"openai"`
	Webhooks     []config.WebhookRule `json:"webhooks"`
	CodeProjects []config.CodeProject `json:"codeProjects"`
}

type accessConfigResponse struct {
	Mode        string `json:"mode"`
	PasswordSet bool   `json:"passwordSet"`
}

type adminAccountResponse struct {
	Username    string `json:"username"`
	PasswordSet bool   `json:"passwordSet"`
}

type apiConfigResponse struct {
	Enabled  bool `json:"enabled"`
	TokenSet bool `json:"tokenSet"`
}

type openAIConfigResponse struct {
	Enabled   bool   `json:"enabled"`
	APIKeySet bool   `json:"apiKeySet"`
	BaseURL   string `json:"baseURL"`
	Model     string `json:"model"`
	Timeout   string `json:"timeout"`
	APIMode   string `json:"apiMode"`
}

func adminConfigResponseFrom(cfg config.Config, configPath string) adminConfigResponse {
	webhooks := cfg.Webhooks
	if webhooks == nil {
		webhooks = []config.WebhookRule{}
	}
	codeProjects := cfg.CodeProjects
	if codeProjects == nil {
		codeProjects = []config.CodeProject{}
	}
	return adminConfigResponse{
		ConfigPath: configPath,
		Server:     cfg.Server,
		SMTP:       cfg.SMTP,
		Mail:       cfg.Mail,
		Access:     accessConfigResponse{Mode: cfg.Access.Mode, PasswordSet: cfg.Access.PasswordHash != ""},
		Admin:      adminAccountResponse{Username: cfg.Admin.Username, PasswordSet: cfg.Admin.PasswordHash != ""},
		API:        apiConfigResponse{Enabled: cfg.API.Enabled, TokenSet: cfg.API.TokenHash != ""},
		OpenAI: openAIConfigResponse{
			Enabled:   cfg.OpenAI.Enabled,
			APIKeySet: strings.TrimSpace(cfg.OpenAI.APIKey) != "" || strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "",
			BaseURL:   cfg.OpenAI.BaseURL,
			Model:     cfg.OpenAI.Model,
			Timeout:   cfg.OpenAI.Timeout,
			APIMode:   cfg.OpenAI.APIMode,
		},
		Webhooks:     webhooks,
		CodeProjects: codeProjects,
	}
}

func writeMailboxAddress(w http.ResponseWriter, status int, local string, domain string) {
	writeJSON(w, status, map[string]string{
		"localPart": local,
		"domain":    domain,
		"address":   local + "@" + domain,
	})
}

func randomLocalPart() string {
	name := randomNameSeeds[randomInt(len(randomNameSeeds))]
	number := 100 + randomInt(9900)
	return name + strconv.Itoa(number)
}

func randomHex(size int) string {
	data := make([]byte, size)
	if _, err := rand.Read(data); err == nil {
		return hex.EncodeToString(data)
	}
	return strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")
}

func randomInt(max int) int {
	if max <= 0 {
		return 0
	}
	value, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return int(time.Now().UnixNano() % int64(max))
	}
	return int(value.Int64())
}

func setSessionCookie(w http.ResponseWriter, name string, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(ttl),
		MaxAge:   int(ttl.Seconds()),
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-LeMail-API-Token")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
