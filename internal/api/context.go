package api

import (
	"context"
	"net/http"

	"github.com/liteploy/liteploy/internal/auth"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey int

const sessionKey contextKey = iota

// withSession attaches session data to a context.
func withSession(ctx context.Context, s *auth.SessionData) context.Context {
	return context.WithValue(ctx, sessionKey, s)
}

// sessionFromContext retrieves session data from a context.
// Returns nil if no session is present.
func sessionFromContext(ctx context.Context) *auth.SessionData {
	s, _ := ctx.Value(sessionKey).(*auth.SessionData)
	return s
}

// handleWebhook delegates to the webhook handler registered on the server.
// The webhook handler validates its own HMAC — no session required.
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// Webhook handler is set up in main; this is a placeholder that calls it.
	if s.webhookHandler != nil {
		s.webhookHandler.ServeHTTP(w, r)
		return
	}
	http.Error(w, "webhooks not configured", http.StatusNotFound)
}
