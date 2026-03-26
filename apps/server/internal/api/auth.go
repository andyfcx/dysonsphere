package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
)

type CredentialValidator interface {
	ValidateCredential(ctx context.Context, hostID, agentToken string) bool
}

type AuthManager struct {
	legacyAgentToken string
	loginUsername    string
	loginPassword    string
	validator        CredentialValidator

	mu       sync.RWMutex
	sessions map[string]string
}

func NewAuthManager(legacyAgentToken, loginUsername, loginPassword string, validator CredentialValidator) *AuthManager {
	return &AuthManager{
		legacyAgentToken: legacyAgentToken,
		loginUsername:    loginUsername,
		loginPassword:    loginPassword,
		validator:        validator,
		sessions:         map[string]string{},
	}
}

func (a *AuthManager) Login(username, password string) (LoginResponse, bool) {
	if !secureCompare(username, a.loginUsername) || !secureCompare(password, a.loginPassword) {
		return LoginResponse{}, false
	}

	token, err := newSessionToken()
	if err != nil {
		return LoginResponse{}, false
	}

	a.mu.Lock()
	a.sessions[token] = username
	a.mu.Unlock()

	return LoginResponse{
		Token:    token,
		Username: username,
	}, true
}

func (a *AuthManager) Logout(token string) {
	if token == "" {
		return
	}
	a.mu.Lock()
	delete(a.sessions, token)
	a.mu.Unlock()
}

func (a *AuthManager) APIAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeError(w, http.StatusUnauthorized, "invalid or missing token")
			return
		}
		if a.isSessionToken(token) {
			next.ServeHTTP(w, r)
			return
		}
		if a.legacyAgentToken != "" && secureCompare(token, a.legacyAgentToken) {
			next.ServeHTTP(w, r)
			return
		}

		hostID := r.Header.Get("X-Host-ID")
		if hostID != "" && a.validator != nil && a.validator.ValidateCredential(r.Context(), hostID, token) {
			next.ServeHTTP(w, r)
			return
		}

		writeError(w, http.StatusUnauthorized, "invalid or missing token")
	})
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func (a *AuthManager) isSessionToken(token string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.sessions[token]
	return ok
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func secureCompare(got, want string) bool {
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
