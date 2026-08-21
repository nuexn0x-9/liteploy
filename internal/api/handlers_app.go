package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/liteploy/liteploy/internal/application"
	"github.com/liteploy/liteploy/internal/storage"
	"github.com/liteploy/liteploy/internal/system"
)

// --- HTML Application Handlers ---

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if s.settingsSvc != nil {
		settings := s.settingsSvc.Get()
		if settings.PrimaryDomain == "" && !settings.SetupSkipped {
			http.Redirect(w, r, "/setup/domain", http.StatusFound)
			return
		}
	}

	apps := s.appSvc.List()
	s.renderPage(w, r, "dashboard.html", map[string]any{
		"Apps":     apps,
		"Settings": s.settingsSvc.Get(),
		"Session":  sessionFromContext(r.Context()),
	})
}

func (s *Server) handleApplicationsList(w http.ResponseWriter, r *http.Request) {
	apps := s.appSvc.List()
	s.renderPage(w, r, "applications.html", map[string]any{
		"Apps":    apps,
		"Session": sessionFromContext(r.Context()),
	})
}

func (s *Server) handleApplicationNew(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, "application_new.html", map[string]any{
		"Session": sessionFromContext(r.Context()),
	})
}

func (s *Server) handleApplicationCreate(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	id := s.appSvc.GenerateID()

	var regAuth *application.RegistryAuth
	regUser := r.FormValue("registry_username")
	regPass := r.FormValue("registry_password")
	if regUser != "" || regPass != "" {
		regAuth = &application.RegistryAuth{
			ServerAddress: r.FormValue("registry_server"),
			Username:      regUser,
			Password:      regPass,
		}
	}

	app := &application.Application{
		ID:   id,
		Name: r.FormValue("name"),
		Port: formInt(r, "port", 3000),
		Source: application.Source{
			Type:           application.SourceType(r.FormValue("source_type")),
			GitURL:         r.FormValue("git_url"),
			GitBranch:      r.FormValue("git_branch"),
			DockerfilePath: r.FormValue("dockerfile_path"),
			GitAuthType:    r.FormValue("git_auth_type"),
			GitToken:       r.FormValue("git_token"),
			GitSSHKey:      r.FormValue("git_ssh_key"),
			ImageRef:       r.FormValue("image_ref"),
			RegistryAuth:   regAuth,
		},
	}

	if err := s.appSvc.Create(r.Context(), app); err != nil {
		s.renderPage(w, r, "application_new.html", map[string]any{
			"Error":   err.Error(),
			"Session": session,
		})
		return
	}

	http.Redirect(w, r, "/applications/"+id, http.StatusFound)
}

func (s *Server) handleApplicationDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, err := s.appSvc.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	deployments := s.depSvc.ListByApp(id)
	env, _ := s.appSvc.GetEnv(id)

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	webhookURL := fmt.Sprintf("%s://%s/api/webhooks/%s", scheme, r.Host, app.ID)

	s.renderPage(w, r, "application_detail.html", map[string]any{
		"App":         app,
		"Deployments": deployments,
		"Env":         env,
		"WebhookURL":  webhookURL,
		"Msg":         r.URL.Query().Get("msg"),
		"Error":       r.URL.Query().Get("error"),
		"Session":     sessionFromContext(r.Context()),
	})
}

func (s *Server) handleApplicationUpdate(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	id := r.PathValue("id")
	app, err := s.appSvc.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	app.Name = r.FormValue("name")
	app.Port = formInt(r, "port", app.Port)
	app.Source.GitURL = r.FormValue("git_url")
	app.Source.GitBranch = r.FormValue("git_branch")
	app.Source.DockerfilePath = r.FormValue("dockerfile_path")
	app.Source.GitAuthType = r.FormValue("git_auth_type")
	
	// Only update token/ssh key if a non-empty value was posted (or if cleared explicitly)
	if token := r.FormValue("git_token"); token != "" {
		app.Source.GitToken = token
	}
	if sshKey := r.FormValue("git_ssh_key"); sshKey != "" {
		app.Source.GitSSHKey = sshKey
	}

	app.Source.ImageRef = r.FormValue("image_ref")
	app.HealthcheckPath = r.FormValue("healthcheck_path")
	regUser := r.FormValue("registry_username")
	regPass := r.FormValue("registry_password")
	if regUser != "" || regPass != "" {
		if app.Source.RegistryAuth == nil {
			app.Source.RegistryAuth = &application.RegistryAuth{}
		}
		app.Source.RegistryAuth.ServerAddress = r.FormValue("registry_server")
		app.Source.RegistryAuth.Username = regUser
		if regPass != "" {
			app.Source.RegistryAuth.Password = regPass
		}
	}

	memMB := formInt(r, "memory_mb", 0)
	if memMB > 0 {
		if app.ResourceLimits == nil {
			app.ResourceLimits = &application.ResourceLimits{}
		}
		app.ResourceLimits.MemoryMB = int64(memMB)
	} else if app.ResourceLimits != nil {
		app.ResourceLimits.MemoryMB = 0
	}

	volHosts := r.Form["vol_host[]"]
	volContainers := r.Form["vol_container[]"]
	var newVols []application.Volume
	for i := 0; i < len(volHosts) && i < len(volContainers); i++ {
		h := strings.TrimSpace(volHosts[i])
		c := strings.TrimSpace(volContainers[i])
		if h != "" && c != "" {
			newVols = append(newVols, application.Volume{HostPath: h, ContainerPath: c})
		}
	}
	app.Volumes = newVols

	if err := s.appSvc.Update(r.Context(), app); err != nil {
		http.Redirect(w, r, "/applications/"+id+"?error="+err.Error(), http.StatusFound)
		return
	}

	http.Redirect(w, r, "/applications/"+id+"?msg=config_updated", http.StatusFound)
}

func (s *Server) handleApplicationDelete(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	id := r.PathValue("id")
	if err := s.appSvc.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/applications", http.StatusFound)
}

func (s *Server) handleApplicationDeploy(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	id := r.PathValue("id")
	if _, err := s.appSvc.Get(id); err != nil {
		http.NotFound(w, r)
		return
	}

	dep, err := s.depSvc.Enqueue(r.Context(), id, "manual")
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	// If HTMX request, return a partial with redirect.
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", fmt.Sprintf("/deployments/%s", dep.ID))
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, "/deployments/"+dep.ID, http.StatusFound)
}

func (s *Server) handleApplicationStart(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	id := r.PathValue("id")
	app, err := s.appSvc.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if s.dockerCli == nil {
		http.Error(w, "Docker service unavailable", http.StatusServiceUnavailable)
		return
	}

	if app.ContainerID == "" {
		http.Error(w, "no container found for application; deploy first", http.StatusBadRequest)
		return
	}

	if err := s.dockerCli.StartContainer(r.Context(), app.ContainerID); err != nil {
		s.logger.Error("start container failed", "app_id", id, "error", err)
		http.Error(w, "failed to start container: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.appSvc.UpdateStatus(app.ID, application.StatusRunning, app.ContainerID)
	http.Redirect(w, r, "/applications/"+id, http.StatusFound)
}

func (s *Server) handleApplicationStop(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	id := r.PathValue("id")
	app, err := s.appSvc.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if s.dockerCli == nil {
		http.Error(w, "Docker service unavailable", http.StatusServiceUnavailable)
		return
	}

	if app.ContainerID == "" {
		http.Error(w, "no container found for application", http.StatusBadRequest)
		return
	}

	if err := s.dockerCli.StopContainer(r.Context(), app.ContainerID, 10); err != nil {
		s.logger.Error("stop container failed", "app_id", id, "error", err)
		http.Error(w, "failed to stop container: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.appSvc.UpdateStatus(app.ID, application.StatusStopped, app.ContainerID)
	http.Redirect(w, r, "/applications/"+id, http.StatusFound)
}

func (s *Server) handleApplicationRestart(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	id := r.PathValue("id")
	app, err := s.appSvc.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if s.dockerCli == nil {
		http.Error(w, "Docker service unavailable", http.StatusServiceUnavailable)
		return
	}

	if app.ContainerID == "" {
		http.Error(w, "no container found for application; deploy first", http.StatusBadRequest)
		return
	}

	_ = s.dockerCli.StopContainer(r.Context(), app.ContainerID, 5)
	if err := s.dockerCli.StartContainer(r.Context(), app.ContainerID); err != nil {
		s.logger.Error("restart container failed", "app_id", id, "error", err)
		http.Error(w, "failed to restart container: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.appSvc.UpdateStatus(app.ID, application.StatusRunning, app.ContainerID)
	http.Redirect(w, r, "/applications/"+id, http.StatusFound)
}

func (s *Server) handleApplicationLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, err := s.appSvc.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	s.renderPage(w, r, "application_logs.html", map[string]any{
		"App":     app,
		"Session": sessionFromContext(r.Context()),
	})
}

func (s *Server) handleDeploymentDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dep, err := s.depSvc.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	s.renderPage(w, r, "deployment_detail.html", map[string]any{
		"Deployment": dep,
		"Session":    sessionFromContext(r.Context()),
	})
}

func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	settings := s.settingsSvc.Get()
	serverIP := settings.ServerIP
	if serverIP == "" {
		serverIP = system.GetServerPublicIP(r.Context())
		if serverIP == "" {
			serverIP = r.Host
			if strings.Contains(serverIP, ":") {
				serverIP = strings.Split(serverIP, ":")[0]
			}
		}
	}

	s.renderPage(w, r, "settings.html", map[string]any{
		"Settings": settings,
		"ServerIP": serverIP,
		"Error":    r.URL.Query().Get("error"),
		"Success":  r.URL.Query().Get("msg") == "success" || r.URL.Query().Get("success") != "",
		"Msg":      r.URL.Query().Get("msg"),
		"Session":  sessionFromContext(r.Context()),
	})
}

func (s *Server) handleSettingsChangePassword(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	oldPw := r.FormValue("old_password")
	newPw := r.FormValue("new_password")
	confirm := r.FormValue("confirm_password")

	if newPw != confirm {
		http.Redirect(w, r, "/settings?error=passwords_do_not_match", http.StatusFound)
		return
	}

	if err := s.userStore.ChangePassword(session.Username, oldPw, newPw); err != nil {
		http.Redirect(w, r, "/settings?error=invalid_password", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/settings?success=password_changed", http.StatusFound)
}

// --- JSON API Handlers ---

func (s *Server) handleAPIListApplications(w http.ResponseWriter, r *http.Request) {
	apps := s.appSvc.List()
	apiOK(w, apps)
}

func (s *Server) handleAPICreateApplication(w http.ResponseWriter, r *http.Request) {
	var app application.Application
	if err := json.NewDecoder(r.Body).Decode(&app); err != nil {
		apiError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	app.ID = s.appSvc.GenerateID()
	if err := s.appSvc.Create(r.Context(), &app); err != nil {
		apiError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	apiOK(w, app)
}

func (s *Server) handleAPIGetApplication(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, err := s.appSvc.Get(id)
	if err != nil {
		apiError(w, "not found", http.StatusNotFound)
		return
	}
	apiOK(w, app)
}

func (s *Server) handleAPIDeleteApplication(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.appSvc.Delete(r.Context(), id); err != nil {
		apiError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAPIDeployApplication(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.appSvc.Get(id); err != nil {
		apiError(w, "not found", http.StatusNotFound)
		return
	}
	dep, err := s.depSvc.Enqueue(r.Context(), id, "api")
	if err != nil {
		apiError(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	apiOK(w, dep)
}

func (s *Server) handleAPIGetDeployment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dep, err := s.depSvc.Get(id)
	if err != nil {
		apiError(w, "not found", http.StatusNotFound)
		return
	}
	apiOK(w, dep)
}

// --- Helpers ---

// formInt parses an integer form field, returning the default on parse failure.
func formInt(r *http.Request, field string, defaultVal int) int {
	var n int
	if _, err := fmt.Sscanf(r.FormValue(field), "%d", &n); err != nil {
		return defaultVal
	}
	return n
}

func (s *Server) handleSystemPrune(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}
	if err := s.dockerCli.PruneAll(r.Context()); err != nil {
		http.Redirect(w, r, "/settings?error="+err.Error(), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/settings?success=system_pruned", http.StatusFound)
}

func (s *Server) handleApplicationStats(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, err := s.appSvc.Get(id)
	if err != nil || app.ContainerID == "" || app.Status != application.StatusRunning {
		fmt.Fprintf(w, "<span class='text-muted'>No live stats</span>")
		return
	}
	stats, err := s.dockerCli.GetContainerStats(r.Context(), app.ContainerID)
	if err != nil {
		fmt.Fprintf(w, "<span class='text-danger'>Stats error</span>")
		return
	}
	
	memMB := float64(stats.MemoryUsageBytes) / 1024 / 1024
	limitMB := float64(stats.MemoryLimitBytes) / 1024 / 1024
	
	html := fmt.Sprintf(`
		<div class="stats-box">
			<div>CPU: <strong>%.2f%%</strong></div>
			<div>RAM: <strong>%.1f / %.1f MB</strong></div>
		</div>
	`, stats.CPUPercent, memMB, limitMB)
	
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func (s *Server) handleApplicationRollback(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}
	
	appID := r.PathValue("id")
	depID := r.PathValue("depID")
	
	_, err := s.depSvc.EnqueueRollback(r.Context(), appID, depID, session.Username)
	if err != nil {
		http.Redirect(w, r, "/deployments?error="+err.Error(), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/deployments", http.StatusFound)
}

func (s *Server) handleClearFailedDeployments(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}
	appID := r.PathValue("id")
	s.depSvc.ClearFailedDeployments(appID)
	http.Redirect(w, r, "/applications/"+appID, http.StatusFound)
}

func (s *Server) handleBackupExport(w http.ResponseWriter, r *http.Request) {
	store, err := storage.New(s.cfg.DataDir)
	if err != nil {
		http.Error(w, "failed to initialize storage: "+err.Error(), http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("liteploy-backup-%s.tar.gz", time.Now().Format("2006-01-02-150405"))
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")

	if err := store.ExportBackup(w); err != nil {
		s.logger.Error("failed to export backup", "error", err)
	}
}

func (s *Server) handleBackupImport(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	// Limit upload size to 50MB
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		http.Redirect(w, r, "/settings?error=invalid_upload", http.StatusFound)
		return
	}

	file, _, err := r.FormFile("backup_file")
	if err != nil {
		http.Redirect(w, r, "/settings?error=no_file_uploaded", http.StatusFound)
		return
	}
	defer file.Close()

	store, err := storage.New(s.cfg.DataDir)
	if err != nil {
		http.Redirect(w, r, "/settings?error="+err.Error(), http.StatusFound)
		return
	}

	if err := store.ImportBackup(file); err != nil {
		http.Redirect(w, r, "/settings?error="+err.Error(), http.StatusFound)
		return
	}

	// Reload applications into memory
	_ = s.appSvc.Reload()

	http.Redirect(w, r, "/settings?success=backup_restored", http.StatusFound)
}