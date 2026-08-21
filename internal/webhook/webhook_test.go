package webhook

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockEnqueuer struct {
	enqueuedAppID string
	branch        string
}

func (m *mockEnqueuer) Enqueue(appID, branch, triggeredBy string) error {
	m.enqueuedAppID = appID
	m.branch = branch
	return nil
}

func TestGitHubWebhookSuccess(t *testing.T) {
	secret := "test-secret-123"
	payload := `{"ref":"refs/heads/main","head_commit":{"id":"abc1234"},"repository":{"clone_url":"https://github.com/org/repo"}}`

	enq := &mockEnqueuer{}
	handler := NewHandler(nil, enq, func(appID string) (string, bool) {
		if appID == "app-001" {
			return secret, true
		}
		return "", false
	})

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/app-001", bytes.NewBufferString(payload))
	req.SetPathValue("app_id", "app-001")
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", "sha256="+hmacSHA256([]byte(secret), []byte(payload)))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if enq.enqueuedAppID != "app-001" {
		t.Errorf("enqueuedAppID = %q, want app-001", enq.enqueuedAppID)
	}
	if enq.branch != "main" {
		t.Errorf("branch = %q, want main", enq.branch)
	}
}

func TestGitHubWebhookInvalidSignature(t *testing.T) {
	secret := "test-secret-123"
	payload := `{"ref":"refs/heads/main"}`

	handler := NewHandler(nil, &mockEnqueuer{}, func(appID string) (string, bool) {
		return secret, true
	})

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/app-001", bytes.NewBufferString(payload))
	req.SetPathValue("app_id", "app-001")
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid-signature")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
	}
}
