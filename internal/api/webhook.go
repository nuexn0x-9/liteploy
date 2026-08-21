package api

import "net/http"

// SetWebhookHandler assigns the webhook handler to the server.
// Called from main after all services are initialized.
func (s *Server) SetWebhookHandler(h http.Handler) {
	s.webhookHandler = h
}
