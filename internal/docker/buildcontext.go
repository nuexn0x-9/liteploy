package docker

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// createBuildContextTar creates an in-memory tar reader from a directory.
// The tar is piped so it is never fully materialized in RAM.
// The caller is responsible for closing the returned ReadCloser.
func createBuildContextTar(dir string) (io.ReadCloser, error) {
	// Verify the directory exists and is actually a directory.
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("build context: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("build context: %s is not a directory", dir)
	}

	pr, pw := io.Pipe()

	go func() {
		tw := tar.NewWriter(pw)
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Compute archive path relative to context dir.
			relPath, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(relPath)

			// Security: do not follow symlinks that escape the context dir.
			// We use Lstat in Walk to detect symlinks.
			if info.Mode()&os.ModeSymlink != 0 {
				link, err := os.Readlink(path)
				if err != nil {
					return err
				}
				// Resolve symlink target.
				target := link
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(path), link)
				}
				target = filepath.Clean(target)
				// Ensure target remains within the context dir.
				contextAbs, _ := filepath.Abs(dir)
				if !strings.HasPrefix(target, contextAbs) {
					// Skip symlinks that escape context; log but don't fail.
					return nil
				}
			}

			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = relPath

			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()

			buf := make([]byte, 32*1024)
			_, err = io.CopyBuffer(tw, f, buf)
			return err
		})

		if err != nil {
			pw.CloseWithError(err)
			return
		}
		tw.Close()
		pw.Close()
	}()

	return pr, nil
}
