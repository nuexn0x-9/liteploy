// Package api provides the HTTP server, router, and handlers for LITEPLOY.
//
// The server exposes two interfaces:
//   - HTML interface: server-rendered pages for the web UI
//   - JSON API: /api/... endpoints for automation and webhooks
//
// Handlers are thin: validate → authenticate → call service → render.
// All business logic lives in the service layer.
package api

import (
	"context"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/liteploy/liteploy/internal/application"
	"github.com/liteploy/liteploy/internal/auth"
	"github.com/liteploy/liteploy/internal/config"
	"github.com/liteploy/liteploy/internal/deployment"
	"github.com/liteploy/liteploy/internal/docker"
	"github.com/liteploy/liteploy/internal/proxy"
	"github.com/liteploy/liteploy/internal/system"
	"github.com/liteploy/liteploy/web"
)

// Server is the HTTP server for LITEPLOY.
type Server struct {
	cfg            *config.Config
	logger         *slog.Logger
	appSvc         *application.Service
	depSvc         *deployment.Service
	authSvc        *auth.Service
	userStore      *auth.UserStore
	settingsSvc    *system.SettingsService
	proxyMgr       *proxy.Manager
	dockerCli      docker.Engine
	templates      *template.Template
	httpServer     *http.Server
	webhookHandler http.Handler
}

// NewServer creates a configured Server.
func NewServer(
	cfg *config.Config,
	logger *slog.Logger,
	appSvc *application.Service,
	depSvc *deployment.Service,
	authSvc *auth.Service,
	userStore *auth.UserStore,
	settingsSvc *system.SettingsService,
	proxyMgr *proxy.Manager,
	dockerCli docker.Engine,
) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		cfg:         cfg,
		logger:      logger,
		appSvc:      appSvc,
		depSvc:      depSvc,
		authSvc:     authSvc,
		userStore:   userStore,
		settingsSvc: settingsSvc,
		proxyMgr:    proxyMgr,
		dockerCli:   dockerCli,
	}

	tmpl, err := s.loadTemplates()
	if err != nil {
		return nil, err
	}
	s.templates = tmpl

	mux := s.buildRouter()

	s.httpServer = &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		// SSE endpoints use a longer timeout; they override via ResponseController.
	}

	return s, nil
}

// Start begins serving HTTP requests.
func (s *Server) Start() error {
	l, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	s.logger.Info("HTTP server listening", "addr", s.httpServer.Addr)
	return s.httpServer.Serve(l)
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Handler returns the underlying http.Handler (used in tests and middleware).
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// buildRouter sets up all routes.
func (s *Server) buildRouter() http.Handler {
	mux := http.NewServeMux()

	// Health endpoints (unauthenticated, lightweight).
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /ready", s.handleReady)

	// Static assets (embedded in binary).
	mux.Handle("GET /static/", http.FileServer(http.FS(web.Assets)))

	// Auth routes (unauthenticated).
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLoginSubmit)
	mux.HandleFunc("POST /logout", s.handleLogout)
	mux.HandleFunc("GET /setup", s.handleSetupPage)
	mux.HandleFunc("POST /setup", s.handleSetupSubmit)

	// Domain Setup Wizard routes (authenticated).
	mux.HandleFunc("GET /setup/domain", s.authenticated(s.handleSetupDomainPage))
	mux.HandleFunc("POST /setup/domain", s.authenticated(s.handleSetupDomainSubmit))
	mux.HandleFunc("GET /setup/domain/dns", s.authenticated(s.handleSetupDomainDNSPage))
	mux.HandleFunc("POST /setup/domain/verify", s.authenticated(s.handleSetupDomainVerify))
	mux.HandleFunc("POST /setup/domain/skip", s.authenticated(s.handleSetupDomainSkip))
	mux.HandleFunc("GET /setup/domain/success", s.authenticated(s.handleSetupDomainSuccessPage))

	// HTML UI routes (authenticated).
	mux.HandleFunc("GET /{$}", s.authenticated(s.handleDashboard))
	mux.HandleFunc("GET /applications", s.authenticated(s.handleApplicationsList))
	mux.HandleFunc("GET /applications/new", s.authenticated(s.handleApplicationNew))
	mux.HandleFunc("POST /applications", s.authenticated(s.handleApplicationCreate))
	mux.HandleFunc("GET /applications/{id}", s.authenticated(s.handleApplicationDetail))
	mux.HandleFunc("POST /applications/{id}", s.authenticated(s.handleApplicationUpdate))
	mux.HandleFunc("POST /applications/{id}/delete", s.authenticated(s.handleApplicationDelete))
	mux.HandleFunc("POST /applications/{id}/deploy", s.authenticated(s.handleApplicationDeploy))
	mux.HandleFunc("POST /applications/{id}/start", s.authenticated(s.handleApplicationStart))
	mux.HandleFunc("POST /applications/{id}/stop", s.authenticated(s.handleApplicationStop))
	mux.HandleFunc("POST /applications/{id}/restart", s.authenticated(s.handleApplicationRestart))
	mux.HandleFunc("POST /applications/{id}/env", s.authenticated(s.handleApplicationUpdateEnv))
	mux.HandleFunc("POST /applications/{id}/domains", s.authenticated(s.handleApplicationAddDomain))
	mux.HandleFunc("POST /applications/{id}/domains/delete", s.authenticated(s.handleApplicationDeleteDomain))
	mux.HandleFunc("GET /applications/{id}/logs", s.authenticated(s.handleApplicationLogs))
	mux.HandleFunc("GET /deployments", s.authenticated(s.handleDeploymentsList))
	mux.HandleFunc("GET /deployments/{id}", s.authenticated(s.handleDeploymentDetail))
	mux.HandleFunc("GET /domains", s.authenticated(s.handleDomainsList))
	mux.HandleFunc("GET /settings", s.authenticated(s.handleSettingsPage))
	mux.HandleFunc("GET /settings/domains", s.authenticated(s.handleSettingsPage))
	mux.HandleFunc("POST /settings/domains/primary", s.authenticated(s.handleSettingsUpdatePrimaryDomain))
	mux.HandleFunc("POST /settings/domains/verify", s.authenticated(s.handleSettingsVerifyDomain))
	mux.HandleFunc("POST /settings/domains/https", s.authenticated(s.handleSettingsToggleHTTPS))
	mux.HandleFunc("POST /settings/password", s.authenticated(s.handleSettingsChangePassword))
	mux.HandleFunc("POST /system/prune", s.authenticated(s.handleSystemPrune))
	mux.HandleFunc("GET /settings/backup/export", s.authenticated(s.handleBackupExport))
	mux.HandleFunc("POST /settings/backup/import", s.authenticated(s.handleBackupImport))
	mux.HandleFunc("GET /applications/{id}/stats", s.authenticated(s.handleApplicationStats))
	mux.HandleFunc("POST /applications/{id}/rollback/{depID}", s.authenticated(s.handleApplicationRollback))
	mux.HandleFunc("POST /applications/{id}/deployments/clear-failed", s.authenticated(s.handleClearFailedDeployments))

	// JSON API routes.
	mux.HandleFunc("GET /api/applications", s.apiAuthenticated(s.handleAPIListApplications))
	mux.HandleFunc("POST /api/applications", s.apiAuthenticated(s.handleAPICreateApplication))
	mux.HandleFunc("GET /api/applications/{id}", s.apiAuthenticated(s.handleAPIGetApplication))
	mux.HandleFunc("DELETE /api/applications/{id}", s.apiAuthenticated(s.handleAPIDeleteApplication))
	mux.HandleFunc("POST /api/applications/{id}/deploy", s.apiAuthenticated(s.handleAPIDeployApplication))
	mux.HandleFunc("GET /api/applications/{id}/env", s.apiAuthenticated(s.handleAPIGetEnv))
	mux.HandleFunc("POST /api/applications/{id}/env", s.apiAuthenticated(s.handleAPIUpdateEnv))
	mux.HandleFunc("GET /api/deployments", s.apiAuthenticated(s.handleAPIListDeployments))
	mux.HandleFunc("GET /api/deployments/{id}", s.apiAuthenticated(s.handleAPIGetDeployment))
	mux.HandleFunc("GET /api/deployments/{id}/events", s.apiAuthenticated(s.handleDeploymentSSE))
	mux.HandleFunc("GET /api/domains", s.apiAuthenticated(s.handleAPIListDomains))
	mux.HandleFunc("GET /api/dns/check", s.apiAuthenticated(s.handleAPIDNSCheck))
	mux.HandleFunc("GET /api/applications/{id}/logs/stream", s.apiAuthenticated(s.handleContainerLogStream))

	// Webhook routes (authenticated by HMAC signature per app, not session).
	mux.HandleFunc("POST /api/webhooks/{app_id}", s.handleWebhook)

	return mux
}

// loadTemplates parses all HTML templates from the embedded filesystem.
func (s *Server) loadTemplates() (*template.Template, error) {
	tmpl := template.New("").Funcs(templateFuncs())
	_, err := tmpl.ParseFS(web.Assets,
		"templates/layouts/*.html",
		"templates/pages/*.html",
	)
	if err != nil {
		return nil, err
	}
	return tmpl, nil
}

// templateFuncs returns the template function map.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
		"formatDuration": func(d float64) string {
			dur := time.Duration(d * float64(time.Second))
			if dur < time.Minute {
				return dur.Round(time.Second).String()
			}
			return dur.Round(time.Second).String()
		},
		"statusColor": func(s string) string {
			switch s {
			case "running", "success":
				return "green"
			case "failed":
				return "red"
			case "deploying", "queued", "building":
				return "yellow"
			default:
				return "gray"
			}
		},
		"countRunning": func(apps []*application.Application) int {
			count := 0
			for _, app := range apps {
				if app.Status == application.StatusRunning {
					count++
				}
			}
			return count
		},
		"minus": func(a, b int) int {
			return a - b
		},
		"shortSHA": func(sha string) string {
			if len(sha) > 7 {
				return sha[:7]
			}
			return sha
		},
		"shortID": func(id string) string {
			if len(id) > 10 {
				return id[:10]
			}
			return id
		},
		"appVersion": func() string {
			return system.Version
		},
	}
}
