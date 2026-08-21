package system

import (
	"context"
	"testing"

	"github.com/liteploy/liteploy/internal/storage"
)

func TestValidateDomain(t *testing.T) {
	valid := []string{
		"example.com",
		"sub.example.com",
		"my-app.co.id",
		"node-1.vps.dev",
		"test1234.org",
	}

	for _, d := range valid {
		if err := ValidateDomain(d); err != nil {
			t.Errorf("ValidateDomain(%q) unexpected error: %v", d, err)
		}
	}

	invalid := []string{
		"",
		"   ",
		"http://example.com",
		"https://example.com",
		"example.com/path",
		"example.com:8080",
		"-badstart.com",
		"badend-.com",
		"foo..bar.com",
		"in valid.com",
		"shell;injection.com",
		"$(whoami).com",
	}

	for _, d := range invalid {
		if err := ValidateDomain(d); err == nil {
			t.Errorf("ValidateDomain(%q) expected error, got nil", d)
		}
	}
}

func TestFormatDashboardDomain(t *testing.T) {
	if got := FormatDashboardDomain("example.com", "liteploy"); got != "liteploy.example.com" {
		t.Fatalf("expected liteploy.example.com, got %s", got)
	}
	if got := FormatDashboardDomain("example.com", ""); got != "liteploy.example.com" {
		t.Fatalf("expected default liteploy.example.com, got %s", got)
	}
	if got := FormatDashboardDomain("", "liteploy"); got != "" {
		t.Fatalf("expected empty, got %s", got)
	}
}

func TestSanitizeHost(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com"},
		{"EXAMPLE.COM", "example.com"},
		{"http://example.com", "example.com"},
		{"https://example.com/dashboard", "example.com"},
		{"example.com:8080", "example.com"},
		{"  SUB.example.COM  ", "sub.example.com"},
	}

	for _, tc := range tests {
		got := SanitizeHost(tc.input)
		if got != tc.expected {
			t.Errorf("SanitizeHost(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestSettingsService_Persistence(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.New(dir)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}

	svc, err := NewSettingsService(store)
	if err != nil {
		t.Fatalf("NewSettingsService: %v", err)
	}

	// Initial check
	initial := svc.Get()
	if initial.PrimaryDomain != "" {
		t.Fatalf("expected empty primary domain initially, got %s", initial.PrimaryDomain)
	}

	// Set primary domain
	if err := svc.SetPrimaryDomain("myserver.dev"); err != nil {
		t.Fatalf("SetPrimaryDomain error: %v", err)
	}

	cur := svc.Get()
	if cur.PrimaryDomain != "myserver.dev" {
		t.Fatalf("expected myserver.dev, got %s", cur.PrimaryDomain)
	}
	if cur.DashboardDomain != "liteploy.myserver.dev" {
		t.Fatalf("expected liteploy.myserver.dev, got %s", cur.DashboardDomain)
	}

	// Reload from storage into new instance to verify atomic JSON write
	svc2, err := NewSettingsService(store)
	if err != nil {
		t.Fatalf("NewSettingsService reload: %v", err)
	}

	cur2 := svc2.Get()
	if cur2.PrimaryDomain != "myserver.dev" || cur2.DashboardDomain != "liteploy.myserver.dev" {
		t.Fatalf("reloaded settings mismatch: %+v", cur2)
	}

	// Skip setup
	if err := svc2.SkipSetup(); err != nil {
		t.Fatalf("SkipSetup: %v", err)
	}
	if !svc2.Get().SetupSkipped {
		t.Fatalf("expected SetupSkipped to be true")
	}
}

func TestVerifyDomainDNS_Invalid(t *testing.T) {
	ctx := context.Background()
	_, err := VerifyDomainDNS(ctx, "invalid domain with spaces", "1.2.3.4")
	if err == nil {
		t.Fatalf("expected error on invalid domain")
	}
}
