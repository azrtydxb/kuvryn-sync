/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/azrtydxb/solder/internal/source"
)

const defaultRevision = "HEAD"

// Cache resolves Git refs by maintaining one local bare clone per canonical URL.
type Cache struct {
	Root string

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewCache creates a Git source cache rooted at root.
func NewCache(root string) *Cache {
	return &Cache{Root: root, locks: map[string]*sync.Mutex{}}
}

// Resolve fetches the repository and resolves the requested ref to a commit SHA.
// GC removes stale cache directories while preserving active resolved source directories.
func (c *Cache) GC(active []source.ResolvedSource, olderThan time.Time) ([]string, error) {
	keep := map[string]struct{}{}
	for _, resolved := range active {
		if resolved.CacheDir != "" {
			keep[resolved.CacheDir] = struct{}{}
		}
	}
	entries, err := os.ReadDir(c.Root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, classified(source.FailureReasonSourceFailure, "Could not inspect source cache for GC", err)
	}
	removed := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(c.Root, entry.Name())
		if _, ok := keep[path]; ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return removed, classified(source.FailureReasonSourceFailure, "Could not inspect source cache entry", err)
		}
		if olderThan.IsZero() || info.ModTime().Before(olderThan) {
			if err := os.RemoveAll(path); err != nil {
				return removed, classified(source.FailureReasonSourceFailure, "Could not remove stale source cache", err)
			}
			removed = append(removed, path)
		}
	}
	return removed, nil
}

func (c *Cache) Resolve(ctx context.Context, repository source.GitRepository) (source.ResolvedSource, error) {
	if strings.TrimSpace(repository.URL) == "" {
		return source.ResolvedSource{}, classified(source.FailureReasonValidationFailure, "Git repository URL is required", nil)
	}
	revision := strings.TrimSpace(repository.Revision)
	if revision == "" {
		revision = defaultRevision
	}
	if err := validateAuth(repository); err != nil {
		return source.ResolvedSource{}, err
	}

	cacheDir := c.cacheDir(repository.URL)
	lock := c.lockFor(cacheDir)
	lock.Lock()
	defer lock.Unlock()

	if err := os.MkdirAll(c.Root, 0o700); err != nil {
		return source.ResolvedSource{}, classified(source.FailureReasonSourceFailure, "Could not create source cache", err)
	}
	if err := c.ensureRepository(ctx, cacheDir, repository); err != nil {
		return source.ResolvedSource{}, err
	}
	if err := c.fetch(ctx, cacheDir, repository); err != nil {
		return source.ResolvedSource{}, err
	}
	commit, err := c.revParse(ctx, cacheDir, repository, revision)
	if err != nil {
		return source.ResolvedSource{}, err
	}
	worktree, err := c.materializeWorktree(ctx, cacheDir, repository, commit)
	if err != nil {
		return source.ResolvedSource{}, err
	}
	return source.ResolvedSource{Revision: commit, CacheDir: worktree}, nil
}

func (c *Cache) lockFor(key string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.locks == nil {
		c.locks = map[string]*sync.Mutex{}
	}
	lock, ok := c.locks[key]
	if !ok {
		lock = &sync.Mutex{}
		c.locks[key] = lock
	}
	return lock
}

func (c *Cache) cacheDir(rawURL string) string {
	sum := sha256.Sum256([]byte(canonicalURL(rawURL)))
	return filepath.Join(c.Root, hex.EncodeToString(sum[:]))
}

func (c *Cache) ensureRepository(ctx context.Context, cacheDir string, repository source.GitRepository) error {
	if _, err := os.Stat(filepath.Join(cacheDir, "HEAD")); err == nil {
		return c.git(ctx, cacheDir, repository, "remote", "set-url", "origin", repository.URL)
	} else if !errors.Is(err, os.ErrNotExist) {
		return classified(source.FailureReasonSourceFailure, "Could not inspect source cache", err)
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return classified(source.FailureReasonSourceFailure, "Could not create repository cache", err)
	}
	if err := c.git(ctx, "", repository, "init", "--bare", cacheDir); err != nil {
		return err
	}
	return c.git(ctx, cacheDir, repository, "remote", "add", "origin", repository.URL)
}

func (c *Cache) fetch(ctx context.Context, cacheDir string, repository source.GitRepository) error {
	return c.git(ctx, cacheDir, repository,
		"fetch", "--prune", "origin",
		"+refs/heads/*:refs/heads/*",
		"+refs/tags/*:refs/tags/*")
}

func (c *Cache) materializeWorktree(ctx context.Context, cacheDir string, repository source.GitRepository, commit string) (string, error) {
	worktree := filepath.Join(cacheDir, "worktrees", commit)
	if _, err := os.Stat(filepath.Join(worktree, ".solder-checkout")); err == nil {
		return worktree, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", classified(source.FailureReasonSourceFailure, "Could not inspect source worktree", err)
	}
	if err := os.RemoveAll(worktree); err != nil {
		return "", classified(source.FailureReasonSourceFailure, "Could not reset source worktree", err)
	}
	if err := os.MkdirAll(worktree, 0o700); err != nil {
		return "", classified(source.FailureReasonSourceFailure, "Could not create source worktree", err)
	}
	if err := c.git(ctx, cacheDir, repository, "--work-tree", worktree, "checkout", "-f", commit, "--", "."); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(worktree, ".solder-checkout"), []byte(commit), 0o600); err != nil {
		return "", classified(source.FailureReasonSourceFailure, "Could not mark source worktree", err)
	}
	return worktree, nil
}

func (c *Cache) revParse(ctx context.Context, cacheDir string, repository source.GitRepository, revision string) (string, error) {
	candidates := []string{revision}
	if !looksLikeCommit(revision) {
		candidates = append(candidates, "refs/heads/"+revision, "refs/tags/"+revision, "origin/"+revision)
	}
	var last error
	for _, candidate := range candidates {
		out, err := c.gitOutput(ctx, cacheDir, repository, "rev-parse", "--verify", candidate+"^{commit}")
		if err == nil {
			return strings.TrimSpace(out), nil
		}
		last = err
	}
	return "", classified(source.FailureReasonSourceFailure, "Could not resolve Git revision", last)
}

func (c *Cache) git(ctx context.Context, dir string, repository source.GitRepository, args ...string) error {
	_, err := c.gitOutput(ctx, dir, repository, args...)
	return err
}

func (c *Cache) gitOutput(ctx context.Context, dir string, repository source.GitRepository, args ...string) (string, error) {
	env, cleanup, err := gitEnv(repository.Auth)
	if err != nil {
		return "", err
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", classifyGitError(safeGitOutput(out), err)
	}
	return string(out), nil
}

func gitEnv(credentials source.Credentials) ([]string, func(), error) {
	env := append([]string{}, os.Environ()...)
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	cleanup := func() {}

	if credentials.SSHKey != "" || credentials.KnownHosts != "" || credentials.Password != "" || credentials.Token != "" {
		dir, err := os.MkdirTemp("", "solder-git-auth-*")
		if err != nil {
			return nil, cleanup, classified(source.FailureReasonAuthenticationFailure, "Could not prepare Git credentials", err)
		}
		cleanup = func() { _ = os.RemoveAll(dir) }

		if credentials.SSHKey != "" || credentials.KnownHosts != "" {
			parts := []string{"ssh", "-o", "BatchMode=yes"}
			if credentials.KnownHosts != "" {
				knownHostsPath := filepath.Join(dir, "known_hosts")
				if err := os.WriteFile(knownHostsPath, []byte(credentials.KnownHosts), 0o600); err != nil {
					cleanup()
					return nil, func() {}, classified(source.FailureReasonAuthenticationFailure, "Could not write Git known_hosts", err)
				}
				parts = append(parts, "-o", "UserKnownHostsFile="+knownHostsPath)
			}
			if credentials.SSHKey != "" {
				keyPath := filepath.Join(dir, "identity")
				if err := os.WriteFile(keyPath, []byte(credentials.SSHKey), 0o600); err != nil {
					cleanup()
					return nil, func() {}, classified(source.FailureReasonAuthenticationFailure, "Could not write Git SSH key", err)
				}
				parts = append(parts, "-i", keyPath)
			}
			env = append(env, "GIT_SSH_COMMAND="+strings.Join(parts, " "))
		}

		if credentials.Password != "" || credentials.Token != "" {
			askpassPath := filepath.Join(dir, "askpass.sh")
			username := credentials.Username
			secret := credentials.Password
			if credentials.Token != "" {
				if username == "" {
					username = "oauth2"
				}
				secret = credentials.Token
			}
			script := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n*Username*) printf '%%s\\n' %q ;;\n*) printf '%%s\\n' %q ;;\nesac\n", username, secret)
			if err := os.WriteFile(askpassPath, []byte(script), 0o700); err != nil {
				cleanup()
				return nil, func() {}, classified(source.FailureReasonAuthenticationFailure, "Could not write Git askpass helper", err)
			}
			env = append(env, "GIT_ASKPASS="+askpassPath)
		}
	}
	return env, cleanup, nil
}

func validateAuth(repository source.GitRepository) error {
	parsed, err := url.Parse(repository.URL)
	if err != nil {
		return classified(source.FailureReasonValidationFailure, "Git repository URL is invalid", err)
	}
	if parsed.Scheme == "ssh" && repository.Auth.SSHKey == "" && parsed.Host != "" && !isLocalPath(repository.URL) {
		return classified(source.FailureReasonAuthenticationFailure, "SSH Git repository requires an SSH private key Secret", nil)
	}
	if strings.HasPrefix(repository.URL, "git@") && repository.Auth.SSHKey == "" {
		return classified(source.FailureReasonAuthenticationFailure, "SSH Git repository requires an SSH private key Secret", nil)
	}
	return nil
}

func canonicalURL(rawURL string) string {
	return strings.TrimSpace(rawURL)
}

func looksLikeCommit(revision string) bool {
	if len(revision) < 7 || len(revision) > 64 {
		return false
	}
	for _, r := range revision {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func isLocalPath(rawURL string) bool {
	return strings.HasPrefix(rawURL, "/") || strings.HasPrefix(rawURL, "./") || strings.HasPrefix(rawURL, "../")
}

func classifyGitError(output string, err error) error {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "authentication") || strings.Contains(lower, "permission denied") || strings.Contains(lower, "could not read username") || strings.Contains(lower, "terminal prompts disabled") {
		return classified(source.FailureReasonAuthenticationFailure, "Git authentication failed", err)
	}
	return classified(source.FailureReasonSourceFailure, "Git command failed", err)
}

func safeGitOutput(output []byte) string {
	text := string(output)
	if len(text) > 512 {
		text = text[:512]
	}
	return text
}

func classified(reason source.FailureReason, message string, err error) error {
	return &source.Error{Reason: reason, Message: message, Err: err}
}

var _ source.Resolver = (*Cache)(nil)

func (c *Cache) String() string {
	return fmt.Sprintf("git cache %s", c.Root)
}
