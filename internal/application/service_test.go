package application

import (
	"context"
	"testing"

	"github.com/liteploy/liteploy/internal/storage"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func makeApp(id, name string) *Application {
	return &Application{
		ID:   id,
		Name: name,
		Port: 3000,
		Source: Source{
			Type:      SourceImage,
			ImageRef:  "nginx:latest",
		},
	}
}

func TestServiceCreateAndGet(t *testing.T) {
	svc := newTestService(t)

	app := makeApp("app-001", "testapp")
	if err := svc.Create(context.Background(), app); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Get("app-001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "testapp" {
		t.Errorf("Name = %q, want testapp", got.Name)
	}
	if got.Status != StatusCreated {
		t.Errorf("Status = %q, want created", got.Status)
	}
}

func TestServiceDuplicateID(t *testing.T) {
	svc := newTestService(t)
	app := makeApp("app-001", "first")
	if err := svc.Create(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	dup := makeApp("app-001", "second")
	if err := svc.Create(context.Background(), dup); err == nil {
		t.Error("Create with duplicate ID should fail")
	}
}

func TestServiceValidation(t *testing.T) {
	svc := newTestService(t)

	// Empty name should fail.
	bad := makeApp("app-001", "")
	if err := svc.Create(context.Background(), bad); err == nil {
		t.Error("Create with empty name should fail")
	}

	// Bad git URL.
	bad2 := &Application{
		ID:   "app-002",
		Name: "test",
		Port: 3000,
		Source: Source{
			Type:   SourceGit,
			GitURL: "file:///etc/passwd",
		},
	}
	if err := svc.Create(context.Background(), bad2); err == nil {
		t.Error("Create with file:// git URL should fail")
	}
}

func TestServiceUpdate(t *testing.T) {
	svc := newTestService(t)
	app := makeApp("app-001", "original")
	if err := svc.Create(context.Background(), app); err != nil {
		t.Fatal(err)
	}

	app.Name = "updated"
	if err := svc.Update(context.Background(), app); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := svc.Get("app-001")
	if got.Name != "updated" {
		t.Errorf("Name = %q, want updated", got.Name)
	}
}

func TestServiceDelete(t *testing.T) {
	svc := newTestService(t)
	app := makeApp("app-001", "todelete")
	if err := svc.Create(context.Background(), app); err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(context.Background(), "app-001"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := svc.Get("app-001"); err == nil {
		t.Error("Get after Delete should fail")
	}
}

func TestServiceGenerateID(t *testing.T) {
	svc := newTestService(t)

	id1 := svc.GenerateID()
	if id1 != "app-001" {
		t.Errorf("first ID = %q, want app-001", id1)
	}

	app := makeApp(id1, "first")
	svc.Create(context.Background(), app)

	id2 := svc.GenerateID()
	if id2 != "app-002" {
		t.Errorf("second ID = %q, want app-002", id2)
	}
}

func TestServicePersistenceAcrossReloads(t *testing.T) {
	dir := t.TempDir()
	store, _ := storage.New(dir)

	// Create with first service instance.
	svc1, _ := NewService(store, nil)
	app := makeApp("app-001", "persistent")
	svc1.Create(context.Background(), app)

	// Load with second service instance (simulates restart).
	svc2, err := NewService(store, nil)
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc2.Get("app-001")
	if err != nil {
		t.Fatalf("Get after reload: %v", err)
	}
	if got.Name != "persistent" {
		t.Errorf("Name = %q after reload, want persistent", got.Name)
	}
}

func TestParseDomainRoute(t *testing.T) {
	tests := []struct {
		input        string
		wantDomain   string
		wantPath     string
		expectErr    bool
	}{
		{"qulineria.my.id", "qulineria.my.id", "/*", false},
		{"qulineria.my.id/api/*", "qulineria.my.id", "/api/*", false},
		{"qulineria.my.id/api", "qulineria.my.id", "/api/*", false},
		{"qulineria.my.id/assets/*", "qulineria.my.id", "/assets/*", false},
		{"https://qulineria.my.id/api/products", "qulineria.my.id", "/api/products/*", false},
		{"http://api.qulineria.my.id", "api.qulineria.my.id", "/*", false},
		{"", "", "", true},
	}

	for _, tt := range tests {
		domain, path, err := ParseDomainRoute(tt.input)
		if tt.expectErr {
			if err == nil {
				t.Errorf("ParseDomainRoute(%q) expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDomainRoute(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if domain != tt.wantDomain {
			t.Errorf("ParseDomainRoute(%q) domain = %q, want %q", tt.input, domain, tt.wantDomain)
		}
		if path != tt.wantPath {
			t.Errorf("ParseDomainRoute(%q) path = %q, want %q", tt.input, path, tt.wantPath)
		}
	}
}

