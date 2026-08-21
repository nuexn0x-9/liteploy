// Package git provides Git repository operations for LITEPLOY deployments.
//
// Security rules:
//   - Never construct shell commands from user input.
//   - Always use exec.CommandContext with separate argument arrays.
//   - Credentials must not appear in logs.
//   - Working directories are cleaned up after use.
package git

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultCloneTimeout = 10 * time.Minute

// CloneOptions configures a git clone operation.
type CloneOptions struct {
	// URL is the repository URL (validated before reaching here).
	URL string

	// Branch is the branch/ref to checkout. Empty means default branch.
	Branch string

	// Depth limits clone history (1 = shallow clone, reduces data transfer).
	// 0 = full clone.
	Depth int

	// TargetDir is the directory to clone into (must not exist yet).
	TargetDir string

	// AuthToken is used for HTTPS authentication (e.g. GitHub PAT).
	// Never logged.
	AuthToken string

	// SSHKey is the raw PEM private key for git@ URLs.
	// Never logged.
	SSHKey string

	// SSHKeyPath is the path to an SSH private key for git@ URLs.
	// Never logged.
	SSHKeyPath string

	// Timeout for the clone operation.
	Timeout time.Duration

	// Progress writer for streaming clone output.
	Progress io.Writer
}

// CloneResult contains metadata from a successful clone.
type CloneResult struct {
	CommitSHA string
	Branch    string
}

// Clone clones a repository into TargetDir.
// Uses exec.CommandContext with separate argument arrays — never shell concatenation.
func Clone(ctx context.Context, opts CloneOptions) (*CloneResult, error) {
	if opts.TargetDir == "" {
		return nil, fmt.Errorf("git clone: target directory is required")
	}
	if !filepath.IsAbs(opts.TargetDir) {
		return nil, fmt.Errorf("git clone: target directory must be absolute, got %q", opts.TargetDir)
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultCloneTimeout
	}

	cloneCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// If a raw SSH key is provided, write it to a temporary file
	sshKeyPath := opts.SSHKeyPath
	if opts.SSHKey != "" {
		tmpFile, err := os.CreateTemp("", "liteploy_git_key_*")
		if err != nil {
			return nil, fmt.Errorf("git clone: create temp ssh key: %w", err)
		}
		defer os.Remove(tmpFile.Name())
		if _, err := tmpFile.WriteString(opts.SSHKey); err != nil {
			tmpFile.Close()
			return nil, fmt.Errorf("git clone: write temp ssh key: %w", err)
		}
		tmpFile.Close()
		_ = os.Chmod(tmpFile.Name(), 0600)
		sshKeyPath = tmpFile.Name()
	}

	// Prepare clone URL with token if HTTPS token auth is provided
	cloneURL := opts.URL
	if opts.AuthToken != "" && strings.HasPrefix(opts.URL, "https://") {
		cloneURL = strings.Replace(opts.URL, "https://", fmt.Sprintf("https://oauth2:%s@", opts.AuthToken), 1)
	}

	// Build clone args — never concatenate user input into a single string.
	args := []string{"clone"}
	if opts.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", opts.Depth))
	}
	if opts.Branch != "" {
		// Validate branch name against dangerous characters.
		if err := validateRef(opts.Branch); err != nil {
			return nil, fmt.Errorf("git clone: %w", err)
		}
		args = append(args, "--branch", opts.Branch)
	}
	args = append(args, "--", cloneURL, opts.TargetDir)

	cmd := exec.CommandContext(cloneCtx, "git", args...)

	// Inject credentials via environment
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if sshKeyPath != "" {
		cmd.Env = append(cmd.Env,
			fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no -o IdentitiesOnly=yes -o BatchMode=yes", sshKeyPath),
		)
	}

	var progress io.Writer = io.Discard
	if opts.Progress != nil {
		if opts.AuthToken != "" {
			progress = &maskWriter{w: opts.Progress, secret: opts.AuthToken}
		} else {
			progress = opts.Progress
		}
	}
	cmd.Stdout = progress
	cmd.Stderr = progress

	if err := cmd.Run(); err != nil {
		if cloneCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("git clone: timed out after %s", timeout)
		}
		return nil, fmt.Errorf("git clone failed (check repository URL, branch, or token permissions)")
	}

	sha, err := revParse(ctx, opts.TargetDir, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git clone: get commit: %w", err)
	}

	branch := opts.Branch
	if branch == "" {
		branch, _ = currentBranch(ctx, opts.TargetDir)
	}

	return &CloneResult{
		CommitSHA: sha,
		Branch:    branch,
	}, nil
}

// Sync clones the repository if TargetDir doesn't exist, or fetches and fast-forwards if it does.
// This significantly speeds up subsequent deployments by reusing git history and maintaining file timestamps for Docker layer caching.
func Sync(ctx context.Context, opts CloneOptions) (*CloneResult, error) {
	gitDir := filepath.Join(opts.TargetDir, ".git")
	if fi, err := os.Stat(gitDir); err != nil || !fi.IsDir() {
		_ = os.RemoveAll(opts.TargetDir)
		return Clone(ctx, opts)
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultCloneTimeout
	}
	syncCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sshKeyPath := opts.SSHKeyPath
	if opts.SSHKey != "" {
		tmpFile, err := os.CreateTemp("", "liteploy_git_key_*")
		if err != nil {
			return nil, fmt.Errorf("git sync: create temp ssh key: %w", err)
		}
		defer os.Remove(tmpFile.Name())
		if _, err := tmpFile.WriteString(opts.SSHKey); err != nil {
			tmpFile.Close()
			return nil, fmt.Errorf("git sync: write temp ssh key: %w", err)
		}
		tmpFile.Close()
		_ = os.Chmod(tmpFile.Name(), 0600)
		sshKeyPath = tmpFile.Name()
	}

	cloneURL := opts.URL
	if opts.AuthToken != "" && strings.HasPrefix(opts.URL, "https://") {
		cloneURL = strings.Replace(opts.URL, "https://", fmt.Sprintf("https://oauth2:%s@", opts.AuthToken), 1)
	}

	branch := opts.Branch
	if branch == "" {
		branch = "main"
	}
	if err := validateRef(branch); err != nil {
		return nil, fmt.Errorf("git sync: %w", err)
	}

	var progress io.Writer = io.Discard
	if opts.Progress != nil {
		if opts.AuthToken != "" {
			progress = &maskWriter{w: opts.Progress, secret: opts.AuthToken}
		} else {
			progress = opts.Progress
		}
	}

	runGit := func(args ...string) error {
		cmd := exec.CommandContext(syncCtx, "git", append([]string{"-C", opts.TargetDir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if sshKeyPath != "" {
			cmd.Env = append(cmd.Env,
				fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no -o IdentitiesOnly=yes -o BatchMode=yes", sshKeyPath),
			)
		}
		cmd.Stdout = progress
		cmd.Stderr = progress
		return cmd.Run()
	}

	// Update remote URL
	_ = runGit("remote", "set-url", "origin", cloneURL)

	// Fetch updates fast
	if err := runGit("fetch", "--depth", "1", "origin", branch); err != nil {
		// If fetch fails, fallback to full fresh clone
		_ = os.RemoveAll(opts.TargetDir)
		return Clone(ctx, opts)
	}

	// Reset hard to origin/branch
	if err := runGit("checkout", "-f", branch); err != nil {
		_ = runGit("checkout", "-B", branch, "origin/"+branch)
	}
	if err := runGit("reset", "--hard", "origin/"+branch); err != nil {
		return nil, fmt.Errorf("git sync reset: %w", err)
	}
	_ = runGit("clean", "-fd")

	sha, err := revParse(ctx, opts.TargetDir, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git sync: get commit: %w", err)
	}

	return &CloneResult{
		CommitSHA: sha,
		Branch:    branch,
	}, nil
}

// revParse runs `git rev-parse <ref>` in the given directory.
func revParse(ctx context.Context, dir, ref string) (string, error) {
	if err := validateRef(ref); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// currentBranch returns the current branch name.
func currentBranch(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// validateRef ensures a git ref/branch name contains no dangerous characters.
// This prevents argument injection even when args are separate.
func validateRef(ref string) error {
	if ref == "" {
		return nil // empty is fine (use default)
	}
	// Disallow characters that have special meaning to git or the OS.
	forbidden := []string{"..", "~", "^", ":", "?", "*", "[", "\\", " ", "\t", "\n", "@{"}
	for _, bad := range forbidden {
		if strings.Contains(ref, bad) {
			return fmt.Errorf("ref %q contains disallowed characters", ref)
		}
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("ref %q must not start with '-'", ref)
	}
	if len(ref) > 256 {
		return fmt.Errorf("ref too long")
	}
	return nil
}

// Cleanup removes a cloned working directory.
// Always call this after a deployment (success or failure) to avoid orphan dirs.
func Cleanup(dir string, root string) error {
	if dir == "" || root == "" {
		return nil
	}
	// Security: ensure dir is under root before removing.
	rel, err := filepath.Rel(root, dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("git cleanup: directory %q is not under root %q", dir, root)
	}
	return os.RemoveAll(dir)
}

// maskWriter intercepts writes and redacts sensitive credentials from logs.
type maskWriter struct {
	w      io.Writer
	secret string
}

func (m *maskWriter) Write(p []byte) (n int, err error) {
	if m.secret == "" {
		return m.w.Write(p)
	}
	s := string(p)
	s = strings.ReplaceAll(s, m.secret, "******")
	_, err = m.w.Write([]byte(s))
	return len(p), err
}
