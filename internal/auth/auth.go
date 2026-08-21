// Package auth handles authentication, session management, and CSRF protection
// for LITEPLOY. It requires no external dependencies beyond the Go standard
// library and golang.org/x/crypto for bcrypt.
//
// Session model: signed, encrypted cookie (no Redis, no database).
// Password hashing: bcrypt (cost 12 — balances security and CPU on low-spec VPS).
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "liteploy_session"
	csrfCookieName    = "liteploy_csrf"
	csrfHeaderName    = "X-CSRF-Token"
	bcryptCost        = 12 // secure for a low-spec VPS; not too high to cause login hangs
	sessionVersion    = 1
)

// Service manages authentication state.
type Service struct {
	secret []byte       // HMAC signing key for sessions
	maxAge time.Duration
	logger *slog.Logger
}

// SessionData holds the payload stored inside a session cookie.
type SessionData struct {
	Version   int       `json:"v"`
	UserID    string    `json:"uid"`
	Username  string    `json:"usr"`
	IssuedAt  time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
	CSRFToken string    `json:"csrf"`
}

// NewService creates an auth Service.
// secret must be a cryptographically random key (≥ 32 bytes).
func NewService(secret []byte, maxAge time.Duration, logger *slog.Logger) (*Service, error) {
	if len(secret) < 32 {
		return nil, errors.New("auth: session secret must be at least 32 bytes")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		secret: secret,
		maxAge: maxAge,
		logger: logger,
	}, nil
}

// HashPassword bcrypt-hashes a password. Returns the hash string for storage.
// Never store plaintext passwords.
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword verifies a plaintext password against a stored bcrypt hash.
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// CreateSession creates a signed session cookie and sets it on the response.
// Also sets a CSRF cookie with a separate token.
func (s *Service) CreateSession(w http.ResponseWriter, r *http.Request, userID, username string) error {
	csrfToken, err := generateToken(32)
	if err != nil {
		return fmt.Errorf("generate csrf token: %w", err)
	}

	now := time.Now().UTC()
	data := SessionData{
		Version:   sessionVersion,
		UserID:    userID,
		Username:  username,
		IssuedAt:  now,
		ExpiresAt: now.Add(s.maxAge),
		CSRFToken: csrfToken,
	}

	value, err := s.encodeSession(data)
	if err != nil {
		return fmt.Errorf("encode session: %w", err)
	}

	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(s.maxAge.Seconds()),
		HttpOnly: true,           // not accessible from JavaScript
		Secure:   secure,         // only sent over HTTPS
		SameSite: http.SameSiteLaxMode,
	})

	// Expose CSRF token in a JS-readable cookie so HTMX can read and send it.
	// SameSite=Lax + non-HttpOnly allows JS (HTMX) to read but not session cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    csrfToken,
		Path:     "/",
		MaxAge:   int(s.maxAge.Seconds()),
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	return nil
}

// DestroySession clears both session and CSRF cookies.
func (s *Service) DestroySession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	http.SetCookie(w, &http.Cookie{
		Name:   csrfCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

// ValidateSession reads and verifies the session cookie.
// Returns the session data or an error if invalid/expired.
func (s *Service) ValidateSession(r *http.Request) (*SessionData, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, errors.New("no session cookie")
	}

	data, err := s.decodeSession(cookie.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid session: %w", err)
	}

	if time.Now().UTC().After(data.ExpiresAt) {
		return nil, errors.New("session expired")
	}
	if data.Version != sessionVersion {
		return nil, errors.New("session version mismatch")
	}

	return data, nil
}

// ValidateCSRF checks that the CSRF token in the request header matches
// the token in the session. Must be called for all state-changing requests.
func (s *Service) ValidateCSRF(r *http.Request, session *SessionData) error {
	// HTMX sends the CSRF token in X-CSRF-Token header.
	// Forms can also use a hidden field (checked below).
	token := r.Header.Get(csrfHeaderName)
	if token == "" {
		// Allow form submissions that include it as a form field.
		token = r.FormValue("_csrf")
	}
	if token == "" {
		return errors.New("csrf: missing token")
	}
	if !hmac.Equal([]byte(token), []byte(session.CSRFToken)) {
		return errors.New("csrf: token mismatch")
	}
	return nil
}

// encodeSession JSON-marshals and HMAC-signs the session data.
// Format: base64(json) + "." + base64(hmac)
func (s *Service) encodeSession(data SessionData) (string, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := s.sign(encoded)

	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac), nil
}

// decodeSession verifies the HMAC and decodes the session payload.
func (s *Service) decodeSession(value string) (*SessionData, error) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("malformed session token")
	}

	encoded, sigStr := parts[0], parts[1]

	sig, err := base64.RawURLEncoding.DecodeString(sigStr)
	if err != nil {
		return nil, errors.New("malformed session signature")
	}

	expected := s.sign(encoded)
	if !hmac.Equal(sig, expected) {
		return nil, errors.New("invalid session signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("malformed session payload")
	}

	var data SessionData
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("decode session data: %w", err)
	}

	return &data, nil
}

// sign produces an HMAC-SHA256 MAC for the given value.
func (s *Service) sign(value string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(value))
	return mac.Sum(nil)
}

// generateToken creates a cryptographically random URL-safe token.
func generateToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
