package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Session struct {
	Kind      string
	Subject   string
	ExpiresAt time.Time
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

func NewSessionManager() *SessionManager {
	return &SessionManager{sessions: make(map[string]Session)}
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPassword(hash string, password string) bool {
	if hash == "" || password == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (m *SessionManager) Create(kind string, subject string, ttl time.Duration) string {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	token := randomToken()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[token] = Session{Kind: kind, Subject: subject, ExpiresAt: time.Now().Add(ttl)}
	return token
}

func (m *SessionManager) Validate(token string, kind string) bool {
	if token == "" {
		return false
	}
	m.mu.RLock()
	session, ok := m.sessions[token]
	m.mu.RUnlock()
	if !ok || session.Kind != kind || time.Now().After(session.ExpiresAt) {
		if ok {
			m.Delete(token)
		}
		return false
	}
	return true
}

func (m *SessionManager) Delete(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
}

func randomToken() string {
	var data [32]byte
	if _, err := rand.Read(data[:]); err == nil {
		return hex.EncodeToString(data[:])
	}
	return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
}
