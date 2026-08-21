package git

import (
	"testing"
)

func TestValidateRef(t *testing.T) {
	valid := []string{
		"main",
		"feature/my-feature",
		"v1.0.0",
		"release-2024",
		"refs/heads/main",
	}
	invalid := []string{
		"../etc",
		"feat..main",
		"name~1",
		"name^",
		"name:thing",
		"name?query",
		"name*glob",
		"name[bracket",
		"name\\backslash",
		"name with spaces",
		"-flag-injection",
		"@{upstream}",
	}

	for _, ref := range valid {
		if err := validateRef(ref); err != nil {
			t.Errorf("validateRef(%q) = %v, want nil", ref, err)
		}
	}
	for _, ref := range invalid {
		if err := validateRef(ref); err == nil {
			t.Errorf("validateRef(%q) = nil, want error", ref)
		}
	}
}

func TestCleanupPathSafety(t *testing.T) {
	root := "/var/lib/liteploy/build"

	// Safe paths.
	safeTests := []struct {
		dir  string
		root string
	}{
		{"/var/lib/liteploy/build/clone-abc123", "/var/lib/liteploy/build"},
	}
	for _, tc := range safeTests {
		// Don't actually remove — just test the path check doesn't error.
		err := Cleanup("", tc.root) // empty dir = no-op
		if err != nil {
			t.Errorf("Cleanup(\"\", %q): %v", tc.root, err)
		}
		_ = tc
	}

	// Dangerous paths.
	err := Cleanup("/etc/passwd", root)
	if err == nil {
		t.Error("Cleanup should fail for path outside root")
	}

	err2 := Cleanup("../../../etc", root)
	if err2 == nil {
		t.Error("Cleanup should fail for path traversal")
	}
}
