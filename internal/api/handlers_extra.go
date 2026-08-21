package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/liteploy/liteploy/internal/application"
	"github.com/liteploy/liteploy/internal/proxy"
)

// --- Deployments List (HTML) ---

func (s *Server) handleDeploymentsList(w http.ResponseWriter, r *http.Request) {
	deployments := s.depSvc.ListAll()
	apps := s.appSvc.List()
	appNames := make(map[string]string)
	for _, a := range apps {
		appNames[a.ID] = a.Name
	}

	s.renderPage(w, r, "deployments.html", map[string]any{
		"Deployments": deployments,
		"AppNames":    appNames,
		"Session":     sessionFromContext(r.Context()),
	})
}

// --- Domains List (HTML) ---

type DomainItem struct {
	Domain   string
	AppID    string
	AppName  string
	Upstream string
	Status   application.Status
}

func (s *Server) handleDomainsList(w http.ResponseWriter, r *http.Request) {
	apps := s.appSvc.List()
	var domainItems []DomainItem

	for _, app := range apps {
		upstream := fmt.Sprintf("liteploy-%s:%d", app.ID, app.Port)
		for _, d := range app.Domains {
			domainItems = append(domainItems, DomainItem{
				Domain:   d,
				AppID:    app.ID,
				AppName:  app.Name,
				Upstream: upstream,
				Status:   app.Status,
			})
		}
	}

	s.renderPage(w, r, "domains.html", map[string]any{
		"Domains": domainItems,
		"Session": sessionFromContext(r.Context()),
	})
}

// --- Environment Variables Handlers ---

func (s *Server) handleApplicationUpdateEnv(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	id := r.PathValue("id")
	if _, err := s.appSvc.Get(id); err != nil {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Read raw env lines from textarea or key-value pairs
	rawEnv := r.FormValue("raw_env")
	envMap := make(map[string]string)

	if rawEnv != "" {
		lines := strings.Split(rawEnv, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				// Strip surrounding quotes if present
				if (strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"")) ||
					(strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'")) {
					v = v[1 : len(v)-1]
				}
				if k != "" {
					envMap[k] = v
				}
			}
		}
	} else {
		// Form keys: env_key[] and env_val[]
		keys := r.Form["env_key[]"]
		vals := r.Form["env_val[]"]
		for i := 0; i < len(keys) && i < len(vals); i++ {
			k := strings.TrimSpace(keys[i])
			v := vals[i]
			if k != "" {
				envMap[k] = v
			}
		}
	}

	if err := s.appSvc.SetEnv(id, envMap); err != nil {
		http.Error(w, "failed to save env: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.logger.Info("application env updated", "app_id", id, "count", len(envMap))
	http.Redirect(w, r, "/applications/"+id+"?msg=env_saved", http.StatusFound)
}

// --- Domain Add/Delete Handlers ---

func (s *Server) handleApplicationAddDomain(w http.ResponseWriter, r *http.Request) {
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

	newDomain := strings.TrimSpace(strings.ToLower(r.FormValue("domain")))
	if newDomain == "" {
		http.Redirect(w, r, "/applications/"+id+"?error=domain_empty", http.StatusFound)
		return
	}

	// Check if domain already exists on app
	for _, d := range app.Domains {
		if d == newDomain {
			http.Redirect(w, r, "/applications/"+id+"?msg=domain_exists", http.StatusFound)
			return
		}
	}

	app.Domains = append(app.Domains, newDomain)
	if err := s.appSvc.Update(r.Context(), app); err != nil {
		http.Redirect(w, r, "/applications/"+id+"?error="+err.Error(), http.StatusFound)
		return
	}

	// Update Caddy routing if app has container running
	if app.Port > 0 {
		containerName := fmt.Sprintf("liteploy-%s-%s", app.ID, app.LastDeploymentID)
		upstream := fmt.Sprintf("%s:%d", containerName, app.Port)
		_ = s.proxyMgr.UpsertRoute(r.Context(), &proxy.Route{
			AppID:    app.ID,
			Domains:  app.Domains,
			Upstream: upstream,
		})
	}

	http.Redirect(w, r, "/applications/"+id+"?msg=domain_added", http.StatusFound)
}

func (s *Server) handleApplicationDeleteDomain(w http.ResponseWriter, r *http.Request) {
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

	toDelete := strings.TrimSpace(r.FormValue("domain"))
	var updatedDomains []string
	for _, d := range app.Domains {
		if d != toDelete {
			updatedDomains = append(updatedDomains, d)
		}
	}

	app.Domains = updatedDomains
	if err := s.appSvc.Update(r.Context(), app); err != nil {
		http.Redirect(w, r, "/applications/"+id+"?error="+err.Error(), http.StatusFound)
		return
	}

	// Update Caddy route
	if len(app.Domains) > 0 && app.Port > 0 {
		containerName := fmt.Sprintf("liteploy-%s-%s", app.ID, app.LastDeploymentID)
		upstream := fmt.Sprintf("%s:%d", containerName, app.Port)
		_ = s.proxyMgr.UpsertRoute(r.Context(), &proxy.Route{
			AppID:    app.ID,
			Domains:  app.Domains,
			Upstream: upstream,
		})
	} else {
		_ = s.proxyMgr.RemoveRoute(r.Context(), app.ID)
	}

	http.Redirect(w, r, "/applications/"+id+"?msg=domain_deleted", http.StatusFound)
}

// --- JSON API Handlers ---

func (s *Server) handleAPIGetEnv(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.appSvc.Get(id); err != nil {
		apiError(w, "application not found", http.StatusNotFound)
		return
	}
	env, err := s.appSvc.GetEnv(id)
	if err != nil {
		apiError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	apiOK(w, env)
}

func (s *Server) handleAPIUpdateEnv(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.appSvc.Get(id); err != nil {
		apiError(w, "application not found", http.StatusNotFound)
		return
	}

	var env map[string]string
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		apiError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.appSvc.SetEnv(id, env); err != nil {
		apiError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	apiOK(w, map[string]any{"status": "ok", "count": len(env)})
}

func (s *Server) handleAPIListDeployments(w http.ResponseWriter, r *http.Request) {
	deployments := s.depSvc.ListAll()
	apiOK(w, deployments)
}

func (s *Server) handleAPIListDomains(w http.ResponseWriter, r *http.Request) {
	apps := s.appSvc.List()
	var domains []map[string]any
	for _, app := range apps {
		for _, d := range app.Domains {
			domains = append(domains, map[string]any{
				"domain":   d,
				"app_id":   app.ID,
				"app_name": app.Name,
				"port":     app.Port,
				"status":   app.Status,
			})
		}
	}
	apiOK(w, domains)
}

// handleAPIDNSCheck checks DNS resolution for a domain
func (s *Server) handleAPIDNSCheck(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		apiError(w, "domain parameter required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var resolver net.Resolver
	ips, err := resolver.LookupHost(ctx, domain)
	if err != nil {
		apiOK(w, map[string]any{
			"domain":   domain,
			"resolved": false,
			"error":    err.Error(),
		})
		return
	}

	apiOK(w, map[string]any{
		"domain":   domain,
		"resolved": true,
		"ips":      ips,
	})
}
