package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var testSecret = []byte("aaaabbbbccccddddeeeeffffgggghhhh") // 32 bytes

func newTestService(t *testing.T) *Service {
	t.Helper()
	svc, err := NewService(testSecret, time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("securepassword123")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword("securepassword123", hash) {
		t.Error("CheckPassword returned false for correct password")
	}
	if CheckPassword("wrongpassword", hash) {
		t.Error("CheckPassword returned true for wrong password")
	}
}

func TestHashPasswordTooShort(t *testing.T) {
	_, err := HashPassword("short")
	if err == nil {
		t.Error("HashPassword should fail for password < 8 chars")
	}
}

func TestSessionRoundTrip(t *testing.T) {
	svc := newTestService(t)

	// Create session.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := svc.CreateSession(rec, req, "user-001", "admin"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Extract cookie from response.
	resp := rec.Result()
	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("session cookie not set")
	}

	// Validate session.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(sessionCookie)
	data, err := svc.ValidateSession(req2)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if data.UserID != "user-001" || data.Username != "admin" {
		t.Errorf("session data mismatch: %+v", data)
	}
}

func TestSessionTamper(t *testing.T) {
	svc := newTestService(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	svc.CreateSession(rec, req, "user-001", "admin")

	resp := rec.Result()
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName {
			// Tamper with the value.
			c.Value = c.Value[:len(c.Value)-4] + "XXXX"
			req2 := httptest.NewRequest(http.MethodGet, "/", nil)
			req2.AddCookie(c)
			_, err := svc.ValidateSession(req2)
			if err == nil {
				t.Error("ValidateSession should fail for tampered cookie")
			}
		}
	}
}

func TestCSRFValidation(t *testing.T) {
	svc := newTestService(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	svc.CreateSession(rec, req, "user-001", "admin")

	resp := rec.Result()
	var session *SessionData
	var csrfToken string
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName {
			req2 := httptest.NewRequest(http.MethodPost, "/", nil)
			req2.AddCookie(c)
			data, _ := svc.ValidateSession(req2)
			session = data
		}
		if c.Name == csrfCookieName {
			csrfToken = c.Value
		}
	}
	if session == nil {
		t.Fatal("could not get session")
	}

	// Valid CSRF.
	req3 := httptest.NewRequest(http.MethodPost, "/", nil)
	req3.Header.Set(csrfHeaderName, csrfToken)
	if err := svc.ValidateCSRF(req3, session); err != nil {
		t.Errorf("ValidateCSRF failed for correct token: %v", err)
	}

	// Invalid CSRF.
	req4 := httptest.NewRequest(http.MethodPost, "/", nil)
	req4.Header.Set(csrfHeaderName, "wrong-token")
	if err := svc.ValidateCSRF(req4, session); err == nil {
		t.Error("ValidateCSRF should fail for wrong token")
	}
}

func TestShortSecret(t *testing.T) {
	_, err := NewService([]byte("short"), time.Hour, nil)
	if err == nil {
		t.Error("NewService should fail with short secret")
	}
}
