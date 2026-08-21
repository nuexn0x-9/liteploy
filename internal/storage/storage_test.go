package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStorePathTraversal(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	// These should all be rejected.
	dangerous := []string{
		"../etc/passwd",
		"../../root/.ssh/authorized_keys",
		"foo/../../etc/passwd",
		"foo/../../../etc",
	}
	for _, p := range dangerous {
		_, err := s.safePath(p)
		if err == nil {
			t.Errorf("safePath(%q) should have returned error", p)
		}
	}
}

func TestStoreWriteReadJSON(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	type sample struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	in := sample{Name: "test", Value: 42}
	if err := s.WriteJSON("config/sample.json", in); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var out sample
	if err := s.ReadJSON("config/sample.json", &out); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}

	if out != in {
		t.Errorf("read %+v, want %+v", out, in)
	}
}

func TestStoreReadNotExist(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	var v any
	err = s.ReadJSON("nonexistent.json", &v)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ReadJSON on missing file: got %v, want os.ErrNotExist", err)
	}
}

func TestAtomicWriteSafety(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	// Write initial content.
	if err := atomicWrite(path, []byte(`"original"`), 0o640); err != nil {
		t.Fatal(err)
	}

	// Overwrite.
	if err := atomicWrite(path, []byte(`"updated"`), 0o640); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"updated"` {
		t.Errorf("got %q, want \"updated\"", string(data))
	}
}

func TestStoreRelativePath(t *testing.T) {
	_, err := New("relative/path")
	if err == nil {
		t.Error("New() with relative path should fail")
	}
}
