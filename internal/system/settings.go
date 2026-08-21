package system

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/liteploy/liteploy/internal/storage"
)

var (
	// domainRegex matches valid domain names (e.g. example.com, sub.example.com, my-app.co.id).
	// Prevents command injection, whitespace, path traversal, or malformed hostnames.
	domainRegex = regexp.MustCompile(`^(?i)[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)
)

// SystemSettings holds server-wide configuration such as Primary Domain and HTTPS status.
type SystemSettings struct {
	PrimaryDomain   string    `json:"primary_domain"`   // e.g. "example.com"
	DashboardDomain string    `json:"dashboard_domain"` // e.g. "liteploy.example.com"
	DashboardSub    string    `json:"dashboard_sub"`    // default "liteploy"
	DNSVerified     bool      `json:"dns_verified"`
	HTTPSEnabled    bool      `json:"https_enabled"`
	ServerIP        string    `json:"server_ip"`
	SetupSkipped    bool      `json:"setup_skipped"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// DNSVerificationResult contains details of a DNS verification check.
type DNSVerificationResult struct {
	Domain            string   `json:"domain"`
	DashboardDomain   string   `json:"dashboard_domain"`
	ExpectedIP        string   `json:"expected_ip"`
	FoundIPs          []string `json:"found_ips"`
	DashboardFoundIPs []string `json:"dashboard_found_ips"`
	ApexVerified      bool     `json:"apex_verified"`
	DashboardVerified bool     `json:"dashboard_verified"`
	Verified          bool     `json:"verified"`
	Error             string   `json:"error,omitempty"`
}

// SettingsService manages system settings and domain verification.
type SettingsService struct {
	store *storage.Store
	mu    sync.RWMutex
	data  SystemSettings
}

const settingsFile = "config/settings.json"

// NewSettingsService creates and initializes a SettingsService.
func NewSettingsService(store *storage.Store) (*SettingsService, error) {
	s := &SettingsService{
		store: store,
		data: SystemSettings{
			DashboardSub: "liteploy",
		},
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	return s, nil
}

// load reads the settings from storage if present.
func (s *SettingsService) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	exists, err := s.store.Exists(settingsFile)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	var data SystemSettings
	if err := s.store.ReadJSON(settingsFile, &data); err != nil {
		return fmt.Errorf("read settings: %w", err)
	}

	if data.DashboardSub == "" {
		data.DashboardSub = "liteploy"
	}
	s.data = data
	return nil
}

// Get returns a copy of current system settings.
func (s *SettingsService) Get() SystemSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

// Save persists the updated system settings.
func (s *SettingsService) Save(settings SystemSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if settings.DashboardSub == "" {
		settings.DashboardSub = "liteploy"
	}
	if settings.PrimaryDomain != "" {
		settings.DashboardDomain = fmt.Sprintf("%s.%s", settings.DashboardSub, settings.PrimaryDomain)
	} else {
		settings.DashboardDomain = ""
	}
	settings.UpdatedAt = time.Now().UTC()

	if err := s.store.WriteJSON(settingsFile, settings); err != nil {
		return fmt.Errorf("persist settings: %w", err)
	}

	s.data = settings
	return nil
}

// SetPrimaryDomain sets and validates a new primary domain.
func (s *SettingsService) SetPrimaryDomain(domain string) error {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain != "" {
		if err := ValidateDomain(domain); err != nil {
			return err
		}
	}

	s.mu.Lock()
	current := s.data
	s.mu.Unlock()

	current.PrimaryDomain = domain
	if domain != "" {
		if current.DashboardSub == "" {
			current.DashboardSub = "liteploy"
		}
		current.DashboardDomain = fmt.Sprintf("%s.%s", current.DashboardSub, domain)
	} else {
		current.DashboardDomain = ""
		current.DNSVerified = false
		current.HTTPSEnabled = false
	}
	current.SetupSkipped = false

	return s.Save(current)
}

// SkipSetup marks the initial domain setup as skipped.
func (s *SettingsService) SkipSetup() error {
	s.mu.Lock()
	current := s.data
	s.mu.Unlock()

	current.SetupSkipped = true
	return s.Save(current)
}

// ValidateDomain validates domain string format.
func ValidateDomain(domain string) error {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return errors.New("domain cannot be empty")
	}
	if len(domain) > 253 {
		return errors.New("domain name exceeds maximum length of 253 characters")
	}
	if strings.Contains(domain, "://") || strings.Contains(domain, "/") || strings.Contains(domain, ":") {
		return errors.New("domain must not contain protocol, path, or port (e.g. use 'example.com')")
	}
	if !domainRegex.MatchString(domain) {
		return errors.New("invalid domain format (e.g. 'example.com' or 'sub.example.com')")
	}
	return nil
}

// FormatDashboardDomain returns the full dashboard hostname for a given primary domain.
func FormatDashboardDomain(primaryDomain, sub string) string {
	primaryDomain = strings.TrimSpace(strings.ToLower(primaryDomain))
	if primaryDomain == "" {
		return ""
	}
	if sub == "" {
		sub = "liteploy"
	}
	return fmt.Sprintf("%s.%s", sub, primaryDomain)
}

// GetServerPublicIP detects the public IP address of the server reliably.
func GetServerPublicIP(ctx context.Context) string {
	client := &http.Client{
		Timeout: 4 * time.Second,
	}

	// Providers to query for public IP
	providers := []string{
		"https://api.ipify.org",
		"https://icanhazip.com",
		"https://ifconfig.me/ip",
		"https://checkip.amazonaws.com",
	}

	for _, provider := range providers {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, provider, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "curl/7.88.1")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
			if err == nil {
				ipStr := strings.TrimSpace(string(body))
				if ip := net.ParseIP(ipStr); ip != nil && ip.To4() != nil {
					return ipStr
				}
			}
		}
	}

	// Fallback: Check local network interfaces for public IPv4
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
					continue
				}
				if ipv4 := ip.To4(); ipv4 != nil {
					return ipv4.String()
				}
			}
		}
	}

	return ""
}

// VerifyDomainDNS checks if the apex domain and/or dashboard domain resolve to expected server IP.
func VerifyDomainDNS(ctx context.Context, domain string, expectedIP string) (*DNSVerificationResult, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if err := ValidateDomain(domain); err != nil {
		return nil, err
	}

	dashboardDomain := "liteploy." + domain
	result := &DNSVerificationResult{
		Domain:          domain,
		DashboardDomain: dashboardDomain,
		ExpectedIP:      expectedIP,
	}

	resolver := &net.Resolver{
		PreferGo: true,
	}

	// 1. Resolve apex domain
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	apexIPs, err := resolver.LookupHost(lookupCtx, domain)
	if err == nil {
		result.FoundIPs = apexIPs
		for _, ip := range apexIPs {
			if expectedIP != "" && ip == expectedIP {
				result.ApexVerified = true
				break
			}
		}
	}

	// 2. Resolve dashboard subdomain
	dashIPs, errDash := resolver.LookupHost(lookupCtx, dashboardDomain)
	if errDash == nil {
		result.DashboardFoundIPs = dashIPs
		for _, ip := range dashIPs {
			if expectedIP != "" && ip == expectedIP {
				result.DashboardVerified = true
				break
			}
		}
	}

	// Overall verified if either dashboard domain or apex domain resolves to server IP
	if result.DashboardVerified || (result.ApexVerified && len(result.DashboardFoundIPs) > 0) {
		result.Verified = true
	} else if expectedIP == "" && (len(apexIPs) > 0 || len(dashIPs) > 0) {
		// If expectedIP unknown, treat successful resolution as verified
		result.Verified = true
	} else {
		result.Verified = false
		if len(result.FoundIPs) == 0 && len(result.DashboardFoundIPs) == 0 {
			result.Error = fmt.Sprintf("DNS records for %s or %s could not be resolved yet", domain, dashboardDomain)
		} else {
			result.Error = fmt.Sprintf("Domain resolves to %v, but expected server IP is %s", append(result.FoundIPs, result.DashboardFoundIPs...), expectedIP)
		}
	}

	return result, nil
}

// SanitizeHost strips protocol and port from user host input.
func SanitizeHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "://") {
		if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
			return strings.ToLower(u.Hostname())
		}
	}
	host, _, err := net.SplitHostPort(raw)
	if err == nil && host != "" {
		return strings.ToLower(host)
	}
	return strings.ToLower(raw)
}
