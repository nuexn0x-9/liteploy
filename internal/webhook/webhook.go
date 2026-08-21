// Package webhook handles incoming webhook requests from Git providers.
//
// Security model:
//   - Each application has its own HMAC-SHA256 webhook secret.
//   - The secret is validated before any payload processing.
//   - The handler returns immediately after enqueueing; no long work is done synchronously.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

const maxWebhookBodySize = 1 * 1024 * 1024 // 1 MB — webhooks should be small

// Provider identifies the Git hosting service.
type Provider string

const (
	ProviderGitHub    Provider = "github"
	ProviderGitLab    Provider = "gitlab"
	ProviderBitbucket Provider = "bitbucket"
)

// Event is a parsed, validated webhook event.
type Event struct {
	Provider  Provider
	AppID     string
	Branch    string
	CommitSHA string
	RepoURL   string
}

// Enqueuer is the interface for triggering a deployment from a webhook.
type Enqueuer interface {
	Enqueue(appID, branch, triggeredBy string) error
}

// Handler processes incoming webhook HTTP requests.
type Handler struct {
	logger   *slog.Logger
	enqueuer Enqueuer
	// getSecret returns the webhook secret for an application ID.
	// Returns ("", false) if the app does not exist or has no webhook.
	getSecret func(appID string) (string, bool)
}

// NewHandler creates a webhook Handler.
func NewHandler(
	logger *slog.Logger,
	enqueuer Enqueuer,
	getSecret func(appID string) (string, bool),
) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		logger:    logger,
		enqueuer:  enqueuer,
		getSecret: getSecret,
	}
}

// ServeHTTP handles POST /api/webhooks/{app-id}.
// It validates the signature, parses the event, and enqueues a deployment.
// The handler always returns quickly — no build work is done synchronously.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract app ID from the URL path. The router sets it in the path.
	appID := r.PathValue("app_id")
	if appID == "" {
		http.Error(w, "missing app_id", http.StatusBadRequest)
		return
	}

	// Get the webhook secret for this application.
	secret, ok := h.getSecret(appID)
	if !ok {
		// Return 404 to avoid leaking that the app exists.
		http.NotFound(w, r)
		return
	}

	// Read body with size limit.
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBodySize))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	// Detect provider and validate signature.
	provider, event, err := h.parseAndValidate(r, body, secret)
	if err != nil {
		h.logger.Warn("webhook validation failed",
			"app_id", appID,
			"error", err.Error(),
			// Never log the signature or body content that might contain secrets.
		)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	event.AppID = appID
	event.Provider = provider

	// Enqueue asynchronously — the webhook must return quickly.
	if err := h.enqueuer.Enqueue(appID, event.Branch, "webhook:"+string(provider)); err != nil {
		h.logger.Error("webhook: enqueue failed", "app_id", appID, "error", err)
		// Return 202 even on enqueue failure to avoid leaking internal state.
		w.WriteHeader(http.StatusAccepted)
		return
	}

	h.logger.Info("webhook: deployment enqueued",
		"app_id", appID,
		"provider", provider,
		"branch", event.Branch,
	)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

// parseAndValidate detects the provider, validates the HMAC, and parses the event.
func (h *Handler) parseAndValidate(r *http.Request, body []byte, secret string) (Provider, *Event, error) {
	// Detect provider from headers.
	switch {
	case r.Header.Get("X-GitHub-Event") != "":
		if err := validateGitHubSignature(r, body, secret); err != nil {
			return "", nil, err
		}
		ev, err := parseGitHubEvent(r, body)
		return ProviderGitHub, ev, err

	case r.Header.Get("X-Gitlab-Event") != "":
		if err := validateGitLabToken(r, secret); err != nil {
			return "", nil, err
		}
		ev, err := parseGitLabEvent(body)
		return ProviderGitLab, ev, err

	default:
		return "", nil, errors.New("unknown webhook provider")
	}
}

// validateGitHubSignature checks X-Hub-Signature-256.
func validateGitHubSignature(r *http.Request, body []byte, secret string) error {
	sig := r.Header.Get("X-Hub-Signature-256")
	if sig == "" {
		return errors.New("missing X-Hub-Signature-256")
	}
	if !strings.HasPrefix(sig, "sha256=") {
		return errors.New("invalid signature format")
	}
	expected := hmacSHA256([]byte(secret), body)
	got := sig[7:] // strip "sha256="

	// Use constant-time comparison to prevent timing attacks.
	if !hmac.Equal([]byte(got), []byte(expected)) {
		return errors.New("signature mismatch")
	}
	return nil
}

// validateGitLabToken checks X-Gitlab-Token.
func validateGitLabToken(r *http.Request, secret string) error {
	token := r.Header.Get("X-Gitlab-Token")
	if !hmac.Equal([]byte(token), []byte(secret)) {
		return errors.New("invalid GitLab token")
	}
	return nil
}

// parseGitHubEvent extracts branch and commit from a GitHub push event.
func parseGitHubEvent(r *http.Request, body []byte) (*Event, error) {
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType != "push" {
		// Ignore non-push events silently.
		return &Event{}, nil
	}

	var payload struct {
		Ref        string `json:"ref"`
		HeadCommit struct {
			ID string `json:"id"`
		} `json:"head_commit"`
		Repository struct {
			CloneURL string `json:"clone_url"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse github event: %w", err)
	}

	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")

	return &Event{
		Branch:    branch,
		CommitSHA: payload.HeadCommit.ID,
		RepoURL:   payload.Repository.CloneURL,
	}, nil
}

// parseGitLabEvent extracts branch and commit from a GitLab push event.
func parseGitLabEvent(body []byte) (*Event, error) {
	var payload struct {
		Ref        string `json:"ref"`
		CheckoutSHA string `json:"checkout_sha"`
		Repository struct {
			GitHTTPURL string `json:"git_http_url"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse gitlab event: %w", err)
	}

	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")

	return &Event{
		Branch:    branch,
		CommitSHA: payload.CheckoutSHA,
		RepoURL:   payload.Repository.GitHTTPURL,
	}, nil
}

// hmacSHA256 computes HMAC-SHA256 of data with key, returned as hex string.
func hmacSHA256(key, data []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}
