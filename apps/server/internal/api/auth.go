package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	sessionTTL       = 24 * time.Hour
	loginMaxAttempts = 5
	loginLockout     = 5 * time.Minute
	cleanupInterval  = 10 * time.Minute
)

type CredentialValidator interface {
	ValidateCredential(ctx context.Context, hostID, agentToken string) bool
}

type sessionEntry struct {
	username  string
	createdAt time.Time
}

type loginAttempt struct {
	failures  int
	lockedAt  time.Time
}

type AuthManager struct {
	legacyAgentToken string
	loginUsername    string
	loginPassword    string
	validator        CredentialValidator

	mu       sync.RWMutex
	sessions map[string]sessionEntry

	attemptsMu sync.Mutex
	attempts   map[string]*loginAttempt // keyed by IP
}

func NewAuthManager(legacyAgentToken, loginUsername, loginPassword string, validator CredentialValidator) *AuthManager {
	a := &AuthManager{
		legacyAgentToken: legacyAgentToken,
		loginUsername:    loginUsername,
		loginPassword:    loginPassword,
		validator:        validator,
		sessions:         map[string]sessionEntry{},
		attempts:         map[string]*loginAttempt{},
	}
	go a.cleanupLoop()
	return a
}

// Login validates credentials against the IP-based lockout, then issues a session token.
// Returns the response and whether login succeeded. On failure, callers should not
// distinguish between wrong credentials and lockout (return 401 for both).
func (a *AuthManager) Login(ip, username, password string) (LoginResponse, bool) {
	if a.isLockedOut(ip) {
		return LoginResponse{}, false
	}

	// Always run both comparisons to avoid short-circuit timing leaks.
	userOK := secureEqual(username, a.loginUsername)
	passOK := secureEqual(password, a.loginPassword)
	if !userOK || !passOK {
		a.recordFailure(ip)
		return LoginResponse{}, false
	}

	a.clearFailures(ip)

	token, err := newSessionToken()
	if err != nil {
		return LoginResponse{}, false
	}

	a.mu.Lock()
	a.sessions[token] = sessionEntry{username: username, createdAt: time.Now()}
	a.mu.Unlock()

	return LoginResponse{Token: token, Username: username}, true
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
		if a.legacyAgentToken != "" && secureEqual(token, a.legacyAgentToken) {
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

// ── lockout ───────────────────────────────────────────────────────────────────

func (a *AuthManager) isLockedOut(ip string) bool {
	a.attemptsMu.Lock()
	defer a.attemptsMu.Unlock()
	att, ok := a.attempts[ip]
	if !ok {
		return false
	}
	if att.failures >= loginMaxAttempts {
		if time.Since(att.lockedAt) < loginLockout {
			return true
		}
		// Lockout expired — reset.
		delete(a.attempts, ip)
	}
	return false
}

func (a *AuthManager) recordFailure(ip string) {
	a.attemptsMu.Lock()
	defer a.attemptsMu.Unlock()
	att := a.attempts[ip]
	if att == nil {
		att = &loginAttempt{}
		a.attempts[ip] = att
	}
	att.failures++
	if att.failures >= loginMaxAttempts {
		att.lockedAt = time.Now()
	}
}

func (a *AuthManager) clearFailures(ip string) {
	a.attemptsMu.Lock()
	delete(a.attempts, ip)
	a.attemptsMu.Unlock()
}

// ── session helpers ───────────────────────────────────────────────────────────

func (a *AuthManager) isSessionToken(token string) bool {
	a.mu.RLock()
	entry, ok := a.sessions[token]
	a.mu.RUnlock()
	if !ok {
		return false
	}
	if time.Since(entry.createdAt) > sessionTTL {
		a.mu.Lock()
		delete(a.sessions, token)
		a.mu.Unlock()
		return false
	}
	return true
}

// cleanupLoop periodically evicts expired sessions and stale attempt records.
func (a *AuthManager) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()

		a.mu.Lock()
		for tok, entry := range a.sessions {
			if now.Sub(entry.createdAt) > sessionTTL {
				delete(a.sessions, tok)
			}
		}
		a.mu.Unlock()

		a.attemptsMu.Lock()
		for ip, att := range a.attempts {
			if att.failures >= loginMaxAttempts && now.Sub(att.lockedAt) >= loginLockout {
				delete(a.attempts, ip)
			} else if att.failures < loginMaxAttempts && now.Sub(att.lockedAt) > loginLockout {
				delete(a.attempts, ip)
			}
		}
		a.attemptsMu.Unlock()
	}
}

// ── misc helpers ──────────────────────────────────────────────────────────────

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// secureEqual compares two strings in constant time regardless of length,
// preventing both content and length timing leaks.
func secureEqual(got, want string) bool {
	// Pad to same length before comparing to avoid early exit on length diff.
	gBytes := []byte(got)
	wBytes := []byte(want)
	// subtle.ConstantTimeEq checks int equality in constant time.
	lenOK := subtle.ConstantTimeEq(int32(len(gBytes)), int32(len(wBytes)))
	// Ensure both slices are the same length for ConstantTimeCompare.
	if len(gBytes) < len(wBytes) {
		gBytes = append(gBytes, make([]byte, len(wBytes)-len(gBytes))...)
	} else if len(wBytes) < len(gBytes) {
		wBytes = append(wBytes, make([]byte, len(gBytes)-len(wBytes))...)
	}
	contentOK := subtle.ConstantTimeCompare(gBytes, wBytes)
	return subtle.ConstantTimeEq(int32(lenOK+contentOK), 2) == 1
}
