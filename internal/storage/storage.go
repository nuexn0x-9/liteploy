// Package storage provides filesystem-based persistence for LITEPLOY.
//
// All writes use an atomic pattern:
//
//	write to temp file → sync → atomic rename
//
// This ensures that a crash during a write never corrupts existing state.
// All data lives under a configurable root directory (default /var/lib/liteploy/).
package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Store manages filesystem persistence for LITEPLOY state.
type Store struct {
	root string
}

// New creates a Store rooted at the given directory.
// The directory and required subdirectories are created if they do not exist.
func New(root string) (*Store, error) {
	// Validate root is an absolute path to prevent accidental relative paths.
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("storage root must be an absolute path, got %q", root)
	}

	s := &Store{root: root}

	// Create required subdirectories.
	dirs := []string{
		root,
		filepath.Join(root, "config"),
		filepath.Join(root, "applications"),
		filepath.Join(root, "logs"),
		filepath.Join(root, "secrets"),
		filepath.Join(root, "state"),
		filepath.Join(root, "backups"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return nil, fmt.Errorf("storage: create dir %s: %w", d, err)
		}
	}

	return s, nil
}

// Root returns the storage root directory.
func (s *Store) Root() string { return s.root }

// WriteJSON atomically writes a JSON-encoded value to the given path within
// the storage root. The path must not contain ".." or escape the root.
func (s *Store) WriteJSON(relPath string, v any) error {
	absPath, err := s.safePath(relPath)
	if err != nil {
		return err
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(absPath), 0o750); err != nil {
		return fmt.Errorf("storage: mkdir %s: %w", filepath.Dir(absPath), err)
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("storage: marshal %s: %w", relPath, err)
	}

	return atomicWrite(absPath, data, 0o640)
}

// ReadJSON reads and JSON-decodes a value from the given path within the storage root.
// Returns os.ErrNotExist if the file does not exist.
func (s *Store) ReadJSON(relPath string, v any) error {
	absPath, err := s.safePath(relPath)
	if err != nil {
		return err
	}

	f, err := os.Open(absPath)
	if err != nil {
		return err // preserves os.ErrNotExist
	}
	defer f.Close()

	// Limit read to 16 MB to avoid loading unreasonably large files into RAM.
	r := io.LimitReader(f, 16*1024*1024)

	if err := json.NewDecoder(r).Decode(v); err != nil {
		return fmt.Errorf("storage: decode %s: %w", relPath, err)
	}
	return nil
}

// WriteBytes atomically writes raw bytes to the given path within the storage root.
func (s *Store) WriteBytes(relPath string, data []byte, perm os.FileMode) error {
	absPath, err := s.safePath(relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o750); err != nil {
		return fmt.Errorf("storage: mkdir %s: %w", filepath.Dir(absPath), err)
	}
	return atomicWrite(absPath, data, perm)
}

// ReadBytes reads raw bytes from the given path within the storage root.
// Reads are bounded to maxBytes to protect memory on a 1 GB VPS.
func (s *Store) ReadBytes(relPath string, maxBytes int64) ([]byte, error) {
	absPath, err := s.safePath(relPath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxBytes))
	if err != nil {
		return nil, fmt.Errorf("storage: read %s: %w", relPath, err)
	}
	return data, nil
}

// Exists reports whether a path within the storage root exists.
func (s *Store) Exists(relPath string) (bool, error) {
	absPath, err := s.safePath(relPath)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(absPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// Remove deletes a file at the given path within the storage root.
func (s *Store) Remove(relPath string) error {
	absPath, err := s.safePath(relPath)
	if err != nil {
		return err
	}
	return os.Remove(absPath)
}

// MkdirAll creates a directory and all parents within the storage root.
func (s *Store) MkdirAll(relPath string) error {
	absPath, err := s.safePath(relPath)
	if err != nil {
		return err
	}
	return os.MkdirAll(absPath, 0o750)
}

// ListDir returns the names of entries in the given directory within the storage root.
func (s *Store) ListDir(relPath string) ([]string, error) {
	absPath, err := s.safePath(relPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

// AbsPath returns the absolute filesystem path for a relative path within the storage root.
// Primarily for passing to external tools that need a real path (e.g., Docker build context).
func (s *Store) AbsPath(relPath string) (string, error) {
	return s.safePath(relPath)
}

// safePath validates relPath and joins it to the storage root.
// It protects against path traversal attacks by ensuring the resolved path
// remains within the storage root.
func (s *Store) safePath(relPath string) (string, error) {
	// Reject obviously dangerous patterns early.
	if strings.Contains(relPath, "..") {
		return "", fmt.Errorf("storage: path %q contains disallowed '..'", relPath)
	}

	// Clean and join.
	joined := filepath.Join(s.root, filepath.FromSlash(relPath))

	// Ensure the resolved path is still under the root.
	// filepath.Rel will fail or produce a path starting with ".." if it escapes.
	rel, err := filepath.Rel(s.root, joined)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("storage: path %q escapes storage root", relPath)
	}

	return joined, nil
}

// atomicWrite writes data to path using a temporary file + rename pattern.
// This ensures that a crash during write cannot corrupt existing state.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	// Create temp file in the same directory so rename is atomic
	// (both source and destination are on the same filesystem).
	tmp, err := os.CreateTemp(dir, ".liteploy-tmp-*")
	if err != nil {
		return fmt.Errorf("atomicWrite: create temp: %w", err)
	}
	tmpName := tmp.Name()

	// Clean up the temp file on any error.
	defer func() {
		if tmp != nil {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("atomicWrite: write temp: %w", err)
	}

	// Set correct permissions before rename.
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("atomicWrite: chmod: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("atomicWrite: sync: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("atomicWrite: close: %w", err)
	}
	tmp = nil // prevent deferred cleanup from double-closing

	// Atomic rename — on POSIX this is guaranteed atomic. On Windows it may
	// fail if the destination is open; we accept this limitation.
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName) // best-effort cleanup
		return fmt.Errorf("atomicWrite: rename: %w", err)
	}

	return nil
}
