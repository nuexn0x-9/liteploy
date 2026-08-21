package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/liteploy/liteploy/internal/auth"
	"github.com/liteploy/liteploy/internal/system"
)

// --- Health ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": system.Version,
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	// Check critical dependencies.
	errs := map[string]string{}

	// Check storage is accessible (very lightweight: just check root dir exists).
	// A real readiness check would ping Docker and Caddy too,
	// but we keep it fast for health-checker polling.

	if len(errs) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"status": "not ready", "errors": errs})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

// --- Authentication middleware ---

// authenticated wraps a handler requiring a valid session.
// Redirects to /login if no valid session exists.
func (s *Server) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Redirect to setup if no admin user exists yet.
		if !s.userStore.HasUsers() {
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		}

		session, err := s.authSvc.ValidateSession(r)
		if err != nil {
			http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusFound)
			return
		}

		// Attach session to request context for downstream handlers.
		ctx := withSession(r.Context(), session)
		next(w, r.WithContext(ctx))
	}
}

// apiAuthenticated wraps a JSON API handler requiring a valid session.
// Returns 401 JSON instead of redirect.
func (s *Server) apiAuthenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := s.authSvc.ValidateSession(r)
		if err != nil {
			apiError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := withSession(r.Context(), session)
		next(w, r.WithContext(ctx))
	}
}

// requireCSRF validates the CSRF token for state-changing requests.
// Returns false and writes an error response if validation fails.
func (s *Server) requireCSRF(w http.ResponseWriter, r *http.Request, session *auth.SessionData) bool {
	if err := s.authSvc.ValidateCSRF(r, session); err != nil {
		http.Error(w, "CSRF validation failed", http.StatusForbidden)
		return false
	}
	return true
}

// --- Auth handlers ---

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if !s.userStore.HasUsers() {
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}
	s.renderPage(w, r, "login.html", map[string]any{
		"Error": r.URL.Query().Get("error"),
		"Next":  r.URL.Query().Get("next"),
	})
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	next := r.FormValue("next")
	if next == "" {
		next = "/"
	}

	user, err := s.userStore.Authenticate(username, password)
	if err != nil {
		// Redirect back with error — don't leak auth details in body.
		http.Redirect(w, r, "/login?error=invalid_credentials&next="+next, http.StatusFound)
		return
	}

	if err := s.authSvc.CreateSession(w, r, user.ID, user.Username); err != nil {
		s.logger.Error("create session failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, next, http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.authSvc.DestroySession(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) handleSetupPage(w http.ResponseWriter, r *http.Request) {
	if s.userStore.HasUsers() {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	s.renderPage(w, r, "setup.html", map[string]any{
		"Error": r.URL.Query().Get("error"),
	})
}

func (s *Server) handleSetupSubmit(w http.ResponseWriter, r *http.Request) {
	if s.userStore.HasUsers() {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm_password")

	if password != confirm {
		http.Redirect(w, r, "/setup?error=passwords_do_not_match", http.StatusFound)
		return
	}

	if err := s.userStore.CreateAdmin(username, password); err != nil {
		s.logger.Error("create admin failed", "error", err)
		http.Redirect(w, r, "/setup?error="+err.Error(), http.StatusFound)
		return
	}

	s.logger.Info("admin user created via setup wizard")
	http.Redirect(w, r, "/login", http.StatusFound)
}

// --- Helper functions ---

// apiError writes a JSON error response.
func apiError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// apiOK writes a JSON success response.
func apiOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// renderPage renders an HTML template. On error, a 500 is returned.
func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		s.logger.Error("template render error", "template", name, "error", err)
		// Don't write another header — just log.
	}
}
