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
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/azrtydxb/solder/internal/source"
)

const (
	defaultRevision = "HEAD"
	// checkoutMarker marks a fully written worktree; no Git entry may use it.
	checkoutMarker = ".solder-checkout"
	// maxSymlinkHops bounds symlink resolution like the kernel's loop limit.
	maxSymlinkHops = 40
)

// allowLocalRepositories permits filesystem paths as repository URLs. Only
// tests enable it: a controller has no business reading Git repositories
// from its own filesystem.
var allowLocalRepositories = false

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
	if !RemoteURL(repository.URL) && !allowLocalRepositories {
		return source.ResolvedSource{}, classified(source.FailureReasonValidationFailure, "Git repository URL must use https, http, ssh, or git", nil)
	}

	cacheDir := c.cacheDir(repository.URL)
	lock := c.lockFor(cacheDir)
	lock.Lock()
	defer lock.Unlock()

	if err := os.MkdirAll(c.Root, 0o700); err != nil {
		return source.ResolvedSource{}, classified(source.FailureReasonSourceFailure, "Could not create source cache", err)
	}
	auth, err := AuthMethod(repository)
	if err != nil {
		return source.ResolvedSource{}, err
	}
	repo, err := c.openRepository(cacheDir, repository)
	if err != nil {
		return source.ResolvedSource{}, err
	}
	if err := c.fetch(ctx, repo, auth); err != nil {
		return source.ResolvedSource{}, err
	}
	commit, err := c.resolve(ctx, repo, auth, revision)
	if err != nil {
		return source.ResolvedSource{}, err
	}
	worktree, err := c.materializeWorktree(repo, cacheDir, commit)
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

func (c *Cache) openRepository(cacheDir string, repository source.GitRepository) (*gogit.Repository, error) {
	repo, err := gogit.PlainOpen(cacheDir)
	if errors.Is(err, gogit.ErrRepositoryNotExists) {
		if err := os.MkdirAll(cacheDir, 0o700); err != nil {
			return nil, classified(source.FailureReasonSourceFailure, "Could not create repository cache", err)
		}
		repo, err = gogit.PlainInit(cacheDir, true)
	}
	if err != nil {
		return nil, classified(source.FailureReasonSourceFailure, "Could not open repository cache", err)
	}
	remote, err := repo.Remote("origin")
	if err == nil && len(remote.Config().URLs) == 1 && remote.Config().URLs[0] == repository.URL {
		return repo, nil
	}
	if err == nil {
		if err := repo.DeleteRemote("origin"); err != nil {
			return nil, classified(source.FailureReasonSourceFailure, "Could not update repository cache remote", err)
		}
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{repository.URL}}); err != nil {
		return nil, classified(source.FailureReasonSourceFailure, "Could not configure repository cache remote", err)
	}
	return repo, nil
}

func (c *Cache) fetch(ctx context.Context, repo *gogit.Repository, auth transport.AuthMethod) error {
	err := repo.FetchContext(ctx, &gogit.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{"+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*"},
		Auth:       auth,
		Tags:       gogit.NoTags,
		Force:      true,
		Prune:      true,
	})
	if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return classifyGitError(err, "Git fetch failed")
	}
	return nil
}

// resolve turns a branch, tag, commit (full or abbreviated), or HEAD for the
// remote's default branch into a commit hash.
func (c *Cache) resolve(ctx context.Context, repo *gogit.Repository, auth transport.AuthMethod, revision string) (string, error) {
	if revision == defaultRevision {
		remote, err := repo.Remote("origin")
		if err != nil {
			return "", classified(source.FailureReasonSourceFailure, "Could not resolve Git revision", err)
		}
		refs, err := remote.ListContext(ctx, &gogit.ListOptions{Auth: auth})
		if err != nil {
			return "", classifyGitError(err, "Could not list Git remote")
		}
		for _, ref := range refs {
			if ref.Name() != plumbing.HEAD {
				continue
			}
			// A remote with a detached HEAD advertises the commit itself.
			if ref.Type() == plumbing.HashReference {
				return ref.Hash().String(), nil
			}
			revision = ref.Target().String()
		}
		// Never fall back to the local cache's own HEAD, which names a branch
		// the remote may not use as its default.
		if revision == defaultRevision {
			return "", classified(source.FailureReasonSourceFailure, "Git remote does not advertise a default branch; set a revision", nil)
		}
	}
	hash, err := repo.ResolveRevision(plumbing.Revision(revision))
	if err != nil {
		return "", classified(source.FailureReasonSourceFailure, "Could not resolve Git revision", err)
	}
	return hash.String(), nil
}

// materializeWorktree writes the commit's files to a directory shared by every
// Application on the same commit, refusing symlinks that resolve outside it.
func (c *Cache) materializeWorktree(repo *gogit.Repository, cacheDir, commit string) (string, error) {
	worktree := filepath.Join(cacheDir, "worktrees", commit)
	if _, err := os.Stat(filepath.Join(worktree, checkoutMarker)); err == nil {
		return worktree, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", classified(source.FailureReasonSourceFailure, "Could not inspect source worktree", err)
	}
	commitObject, err := repo.CommitObject(plumbing.NewHash(commit))
	if err != nil {
		return "", classified(source.FailureReasonSourceFailure, "Could not read Git commit", err)
	}
	tree, err := commitObject.Tree()
	if err != nil {
		return "", classified(source.FailureReasonSourceFailure, "Could not read Git tree", err)
	}
	if err := os.MkdirAll(filepath.Dir(worktree), 0o700); err != nil {
		return "", classified(source.FailureReasonSourceFailure, "Could not create source worktree", err)
	}
	staging, err := os.MkdirTemp(filepath.Dir(worktree), commit+"-*")
	if err != nil {
		return "", classified(source.FailureReasonSourceFailure, "Could not create source worktree", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	err = tree.Files().ForEach(func(file *object.File) error { return writeFile(staging, file) })
	if err == nil {
		// Each link passed on its own; together they may still form a chain
		// that leaves the checkout, which only the finished tree shows.
		err = checkSymlinks(staging)
	}
	if err != nil {
		var sourceErr *source.Error
		if errors.As(err, &sourceErr) {
			return "", err
		}
		return "", classified(source.FailureReasonSourceFailure, "Could not check out Git commit", err)
	}
	if err := createFile(filepath.Join(staging, checkoutMarker), 0o600, strings.NewReader(commit)); err != nil {
		return "", classified(source.FailureReasonSourceFailure, "Could not mark source worktree", err)
	}
	if err := os.RemoveAll(worktree); err != nil {
		return "", classified(source.FailureReasonSourceFailure, "Could not reset source worktree", err)
	}
	if err := os.Rename(staging, worktree); err != nil {
		return "", classified(source.FailureReasonSourceFailure, "Could not publish source worktree", err)
	}
	return worktree, nil
}

func writeFile(root string, file *object.File) error {
	path := filepath.Join(root, filepath.FromSlash(file.Name))
	if !inside(root, path) {
		return classified(source.FailureReasonValidationFailure, fmt.Sprintf("Git path %s escapes the checkout", file.Name), nil)
	}
	if first, _, _ := strings.Cut(file.Name, "/"); strings.EqualFold(first, checkoutMarker) {
		return classified(source.FailureReasonValidationFailure, fmt.Sprintf("Git path %s is reserved", file.Name), nil)
	}
	// A link written earlier may lead a parent directory out of the checkout.
	if ok, err := resolvesInside(root, path); err != nil {
		return err
	} else if !ok {
		return classified(source.FailureReasonValidationFailure, fmt.Sprintf("Git path %s escapes the checkout through a symlink", file.Name), nil)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	switch file.Mode {
	case filemode.Symlink:
		contents, err := file.Contents()
		if err != nil {
			return err
		}
		target := filepath.FromSlash(contents)
		if filepath.IsAbs(target) || !inside(root, filepath.Join(filepath.Dir(path), target)) {
			return classified(source.FailureReasonValidationFailure, fmt.Sprintf("Git symlink %s points outside the checkout", file.Name), nil)
		}
		return os.Symlink(target, path)
	case filemode.Executable:
		return writeBlob(path, 0o700, file)
	default:
		return writeBlob(path, 0o600, file)
	}
}

func writeBlob(path string, perm os.FileMode, file *object.File) error {
	reader, err := file.Reader()
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	return createFile(path, perm, reader)
}

// createFile writes a new file and fails if anything, a symlink included,
// already exists at path, so nothing is ever written through a link.
func createFile(path string, perm os.FileMode, contents io.Reader) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, contents); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// checkSymlinks refuses any symlink under root that resolves outside it.
func checkSymlinks(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.Type()&fs.ModeSymlink == 0 {
			return err
		}
		ok, err := resolvesInside(root, path)
		if err != nil {
			return err
		}
		if !ok {
			rel, _ := filepath.Rel(root, path)
			return classified(source.FailureReasonValidationFailure, fmt.Sprintf("Git symlink %s points outside the checkout", filepath.ToSlash(rel)), nil)
		}
		return nil
	})
}

// resolvesInside follows path below root one component at a time, resolving
// every symlink on the way, and reports whether each step stays inside root.
// A component that does not exist counts as inside: the kernel cannot
// traverse it either, so dangling links are harmless.
func resolvesInside(root, path string) (bool, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil || !inside(root, path) {
		return false, nil
	}
	pending := strings.Split(rel, string(filepath.Separator))
	current := root
	for hops := 0; len(pending) > 0; {
		name := pending[0]
		pending = pending[1:]
		switch name {
		case "", ".":
			continue
		case "..":
			if current == root {
				return false, nil
			}
			current = filepath.Dir(current)
			continue
		}
		next := filepath.Join(current, name)
		info, err := os.Lstat(next)
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			current = next
			continue
		}
		if hops++; hops > maxSymlinkHops {
			return false, nil
		}
		target, err := os.Readlink(next)
		if err != nil {
			return false, err
		}
		if filepath.IsAbs(target) {
			return false, nil
		}
		pending = append(strings.Split(target, string(filepath.Separator)), pending...)
	}
	return true, nil
}

func inside(root, path string) bool {
	rel, err := filepath.Rel(root, filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// AuthMethod builds go-git credentials. SSH requires known_hosts so an
// unknown or changed host key is never trusted.
func AuthMethod(repository source.GitRepository) (transport.AuthMethod, error) {
	credentials := repository.Auth
	if isSSH(repository.URL) {
		endpoint, err := transport.NewEndpoint(repository.URL)
		if err != nil {
			return nil, classified(source.FailureReasonValidationFailure, "Git repository URL is invalid", err)
		}
		user := endpoint.User
		if user == "" {
			user = "git"
		}
		if credentials.SSHKey == "" {
			return nil, classified(source.FailureReasonAuthenticationFailure, "SSH Git repository requires an SSH private key Secret", nil)
		}
		keys, err := ssh.NewPublicKeys(user, []byte(credentials.SSHKey), "")
		if err != nil {
			return nil, classified(source.FailureReasonAuthenticationFailure, "Git SSH private key is invalid", err)
		}
		if strings.TrimSpace(credentials.KnownHosts) == "" {
			return nil, classified(source.FailureReasonAuthenticationFailure, "SSH Git repository requires known_hosts in the credentials Secret", nil)
		}
		callback, err := knownHostsCallback(credentials.KnownHosts)
		if err != nil {
			return nil, err
		}
		keys.HostKeyCallback = callback
		return keys, nil
	}
	if credentials.Token != "" {
		username := credentials.Username
		if username == "" {
			username = "oauth2"
		}
		return &githttp.BasicAuth{Username: username, Password: credentials.Token}, nil
	}
	if credentials.Password != "" {
		return &githttp.BasicAuth{Username: credentials.Username, Password: credentials.Password}, nil
	}
	return nil, nil
}

func knownHostsCallback(knownHosts string) (gossh.HostKeyCallback, error) {
	file, err := os.CreateTemp("", "solder-known-hosts-*")
	if err != nil {
		return nil, classified(source.FailureReasonAuthenticationFailure, "Could not prepare Git known_hosts", err)
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err := file.WriteString(knownHosts); err != nil {
		_ = file.Close()
		return nil, classified(source.FailureReasonAuthenticationFailure, "Could not prepare Git known_hosts", err)
	}
	if err := file.Close(); err != nil {
		return nil, classified(source.FailureReasonAuthenticationFailure, "Could not prepare Git known_hosts", err)
	}
	callback, err := knownhosts.New(file.Name())
	if err != nil {
		return nil, classified(source.FailureReasonAuthenticationFailure, "Git known_hosts is invalid", err)
	}
	return callback, nil
}

// RemoteURL reports whether rawURL uses a network transport Solder accepts.
func RemoteURL(rawURL string) bool {
	for _, scheme := range []string{"https://", "http://", "ssh://", "git://"} {
		if strings.HasPrefix(rawURL, scheme) {
			return true
		}
	}
	return isSSH(rawURL)
}

func isSSH(rawURL string) bool {
	return strings.HasPrefix(rawURL, "ssh://") || (strings.Contains(rawURL, "@") && strings.Contains(rawURL, ":") && !strings.Contains(rawURL, "://"))
}

func canonicalURL(rawURL string) string {
	return strings.TrimSpace(rawURL)
}

func classifyGitError(err error, message string) error {
	if errors.Is(err, transport.ErrAuthenticationRequired) || errors.Is(err, transport.ErrAuthorizationFailed) {
		return classified(source.FailureReasonAuthenticationFailure, "Git authentication failed", err)
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "unable to authenticate") || strings.Contains(lower, "knownhosts") || strings.Contains(lower, "host key") {
		return classified(source.FailureReasonAuthenticationFailure, "Git authentication failed", err)
	}
	return classified(source.FailureReasonSourceFailure, message, err)
}

func classified(reason source.FailureReason, message string, err error) error {
	return &source.Error{Reason: reason, Message: message, Err: err}
}

var _ source.Resolver = (*Cache)(nil)

func (c *Cache) String() string {
	return fmt.Sprintf("git cache %s", c.Root)
}
