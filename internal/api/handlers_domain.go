package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/liteploy/liteploy/internal/system"
)

// --- Initial Setup Domain Wizard Handlers ---

func (s *Server) handleSetupDomainPage(w http.ResponseWriter, r *http.Request) {
	settings := s.settingsSvc.Get()
	serverIP := system.GetServerPublicIP(r.Context())
	if serverIP == "" {
		serverIP = r.Host
		if strings.Contains(serverIP, ":") {
			serverIP = strings.Split(serverIP, ":")[0]
		}
	}

	s.renderPage(w, r, "setup_domain.html", map[string]any{
		"Step":            "input",
		"PrimaryDomain":   settings.PrimaryDomain,
		"DashboardDomain": settings.DashboardDomain,
		"ServerIP":        serverIP,
		"Error":           r.URL.Query().Get("error"),
		"Session":         sessionFromContext(r.Context()),
	})
}

func (s *Server) handleSetupDomainSubmit(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/setup/domain?error=invalid_form", http.StatusFound)
		return
	}

	rawDomain := strings.TrimSpace(r.FormValue("primary_domain"))
	cleanDomain := system.SanitizeHost(rawDomain)

	if err := system.ValidateDomain(cleanDomain); err != nil {
		http.Redirect(w, r, "/setup/domain?error="+err.Error(), http.StatusFound)
		return
	}

	if err := s.settingsSvc.SetPrimaryDomain(cleanDomain); err != nil {
		http.Redirect(w, r, "/setup/domain?error="+err.Error(), http.StatusFound)
		return
	}

	http.Redirect(w, r, "/setup/domain/dns", http.StatusFound)
}

func (s *Server) handleSetupDomainDNSPage(w http.ResponseWriter, r *http.Request) {
	settings := s.settingsSvc.Get()
	if settings.PrimaryDomain == "" {
		http.Redirect(w, r, "/setup/domain", http.StatusFound)
		return
	}

	serverIP := settings.ServerIP
	if serverIP == "" {
		serverIP = system.GetServerPublicIP(r.Context())
		if serverIP == "" {
			serverIP = r.Host
			if strings.Contains(serverIP, ":") {
				serverIP = strings.Split(serverIP, ":")[0]
			}
		}
		settings.ServerIP = serverIP
		_ = s.settingsSvc.Save(settings)
	}

	s.renderPage(w, r, "setup_domain.html", map[string]any{
		"Step":            "dns",
		"PrimaryDomain":   settings.PrimaryDomain,
		"DashboardDomain": settings.DashboardDomain,
		"ServerIP":        serverIP,
		"Error":           r.URL.Query().Get("error"),
		"Session":         sessionFromContext(r.Context()),
	})
}

func (s *Server) handleSetupDomainVerify(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	settings := s.settingsSvc.Get()
	if settings.PrimaryDomain == "" {
		http.Redirect(w, r, "/setup/domain", http.StatusFound)
		return
	}

	serverIP := settings.ServerIP
	if serverIP == "" {
		serverIP = system.GetServerPublicIP(r.Context())
	}

	// Verify DNS
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	result, err := system.VerifyDomainDNS(ctx, settings.PrimaryDomain, serverIP)
	if err != nil {
		http.Redirect(w, r, "/setup/domain/dns?error="+err.Error(), http.StatusFound)
		return
	}

	if !result.Verified {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "DNS verification failed. Please ensure A records point to " + serverIP
		}
		http.Redirect(w, r, "/setup/domain/dns?error="+errMsg, http.StatusFound)
		return
	}

	// DNS Verified! Update settings
	settings.DNSVerified = true
	settings.HTTPSEnabled = true
	settings.ServerIP = serverIP
	if err := s.settingsSvc.Save(settings); err != nil {
		s.logger.Error("failed to save verified settings", "error", err)
	}

	// Configure Caddy reverse proxy for dashboard (liteploy.<domain> -> 127.0.0.1:8080)
	target := s.cfg.HTTPAddr
	if strings.HasPrefix(target, ":") {
		target = "127.0.0.1" + target
	}
	if err := s.proxyMgr.SetDashboardRoute(r.Context(), settings.DashboardDomain, target); err != nil {
		s.logger.Warn("caddy dashboard route update warning", "error", err)
	}

	http.Redirect(w, r, "/setup/domain/success", http.StatusFound)
}

func (s *Server) handleSetupDomainSkip(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	_ = s.settingsSvc.SkipSetup()
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleSetupDomainSuccessPage(w http.ResponseWriter, r *http.Request) {
	settings := s.settingsSvc.Get()
	if settings.PrimaryDomain == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	s.renderPage(w, r, "setup_domain.html", map[string]any{
		"Step":            "success",
		"PrimaryDomain":   settings.PrimaryDomain,
		"DashboardDomain": settings.DashboardDomain,
		"ServerIP":        settings.ServerIP,
		"Session":         sessionFromContext(r.Context()),
	})
}

// --- Settings Domain Management Handlers ---

func (s *Server) handleSettingsUpdatePrimaryDomain(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/settings?error=invalid_form", http.StatusFound)
		return
	}

	rawDomain := strings.TrimSpace(r.FormValue("primary_domain"))
	cleanDomain := system.SanitizeHost(rawDomain)

	if cleanDomain != "" {
		if err := system.ValidateDomain(cleanDomain); err != nil {
			http.Redirect(w, r, "/settings?error="+err.Error(), http.StatusFound)
			return
		}
	}

	settings := s.settingsSvc.Get()
	oldDomain := settings.PrimaryDomain
	oldDashboard := settings.DashboardDomain

	if err := s.settingsSvc.SetPrimaryDomain(cleanDomain); err != nil {
		http.Redirect(w, r, "/settings?error="+err.Error(), http.StatusFound)
		return
	}

	// If domain was removed or changed, adjust Caddy route safely
	if cleanDomain == "" {
		_ = s.proxyMgr.RemoveDashboardRoute(r.Context())
	} else if cleanDomain != oldDomain {
		settings = s.settingsSvc.Get()
		// Test and set new dashboard route if previous was enabled
		target := s.cfg.HTTPAddr
		if strings.HasPrefix(target, ":") {
			target = "127.0.0.1" + target
		}
		if err := s.proxyMgr.SetDashboardRoute(r.Context(), settings.DashboardDomain, target); err != nil {
			s.logger.Warn("caddy update warning for new domain, reverting", "error", err)
			// Rollback domain if caddy failed completely
			_ = s.settingsSvc.SetPrimaryDomain(oldDomain)
			_ = s.proxyMgr.SetDashboardRoute(r.Context(), oldDashboard, target)
			http.Redirect(w, r, "/settings?error=caddy_route_failed", http.StatusFound)
			return
		}
	}

	http.Redirect(w, r, "/settings?msg=domain_updated", http.StatusFound)
}

func (s *Server) handleSettingsVerifyDomain(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	settings := s.settingsSvc.Get()
	if settings.PrimaryDomain == "" {
		http.Redirect(w, r, "/settings?error=no_primary_domain", http.StatusFound)
		return
	}

	serverIP := system.GetServerPublicIP(r.Context())
	if serverIP == "" {
		serverIP = settings.ServerIP
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	result, err := system.VerifyDomainDNS(ctx, settings.PrimaryDomain, serverIP)
	if err != nil {
		http.Redirect(w, r, "/settings?error="+err.Error(), http.StatusFound)
		return
	}

	settings.DNSVerified = result.Verified
	settings.ServerIP = serverIP
	if result.Verified {
		settings.HTTPSEnabled = true
		// Ensure Caddy route is up
		target := s.cfg.HTTPAddr
		if strings.HasPrefix(target, ":") {
			target = "127.0.0.1" + target
		}
		_ = s.proxyMgr.SetDashboardRoute(r.Context(), settings.DashboardDomain, target)
	}
	_ = s.settingsSvc.Save(settings)

	if result.Verified {
		http.Redirect(w, r, "/settings?msg=dns_verified", http.StatusFound)
	} else {
		errStr := result.Error
		if errStr == "" {
			errStr = "DNS verification failed"
		}
		http.Redirect(w, r, "/settings?error="+errStr, http.StatusFound)
	}
}

func (s *Server) handleSettingsToggleHTTPS(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if !s.requireCSRF(w, r, session) {
		return
	}

	settings := s.settingsSvc.Get()
	if settings.PrimaryDomain == "" {
		http.Redirect(w, r, "/settings?error=no_primary_domain", http.StatusFound)
		return
	}

	target := s.cfg.HTTPAddr
	if strings.HasPrefix(target, ":") {
		target = "127.0.0.1" + target
	}

	if settings.HTTPSEnabled {
		// Disable
		settings.HTTPSEnabled = false
		_ = s.proxyMgr.RemoveDashboardRoute(r.Context())
	} else {
		// Enable
		settings.HTTPSEnabled = true
		_ = s.proxyMgr.SetDashboardRoute(r.Context(), settings.DashboardDomain, target)
	}

	_ = s.settingsSvc.Save(settings)
	http.Redirect(w, r, "/settings?msg=https_toggled", http.StatusFound)
}
