package storage

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExportBackup writes a gzipped tar archive of all applications, users, and config to w.
// State and temp build directories are excluded to keep the backup lightweight.
func (s *Store) ExportBackup(w io.Writer) error {
	gw := gzip.NewWriter(w)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Folders to include in backup
	includeDirs := []string{"applications", "users", "config"}

	for _, relDir := range includeDirs {
		absDir := filepath.Join(s.root, relDir)
		if _, err := os.Stat(absDir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Exclude temporary build state / git repos to keep backup ultra small (< 100KB)
			if strings.Contains(path, filepath.Join("applications", "state")) ||
				strings.Contains(path, filepath.Join("applications", "repos")) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			relPath, err := filepath.Rel(s.root, path)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(relPath)

			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}
			header.Name = relPath

			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			if info.Mode().IsRegular() {
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				defer f.Close()
				if _, err := io.Copy(tw, f); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("export %s: %w", relDir, err)
		}
	}

	return nil
}

// ImportBackup extracts a gzipped tar backup archive safely into the storage root.
func (s *Store) ImportBackup(r io.Reader) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("invalid gzip archive: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		// Security: prevent Zip Slip / path traversal
		cleanName := filepath.Clean(header.Name)
		if strings.HasPrefix(cleanName, "..") || strings.HasPrefix(cleanName, "/") || strings.HasPrefix(cleanName, "\\") {
			continue
		}

		targetPath := filepath.Join(s.root, cleanName)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return fmt.Errorf("create dir %s: %w", targetPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("create parent dir for %s: %w", targetPath, err)
			}
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, header.FileInfo().Mode())
			if err != nil {
				return fmt.Errorf("create file %s: %w", targetPath, err)
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return fmt.Errorf("write file %s: %w", targetPath, err)
			}
			outFile.Close()
		}
	}

	return nil
}
