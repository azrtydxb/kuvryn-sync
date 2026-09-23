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
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
	gossh "golang.org/x/crypto/ssh"

	"github.com/azrtydxb/solder/internal/source"
)

func init() {
	// Serve file:// in process so tests need no git binary, and allow the
	// local repositories they create.
	client.InstallProtocol("file", server.DefaultServer)
	allowLocalRepositories = true
}

func TestCacheResolvesBranchesTagsAndCommits(t *testing.T) {
	ctx := context.Background()
	repoDir, commit := createGitRepository(t)
	cache := NewCache(filepath.Join(t.TempDir(), "cache"))

	branch, err := cache.Resolve(ctx, source.GitRepository{URL: repoDir, Revision: "main"})
	if err != nil {
		t.Fatalf("resolve branch: %v", err)
	}
	if branch.Revision != commit {
		t.Fatalf("branch revision = %q, want %q", branch.Revision, commit)
	}

	tag, err := cache.Resolve(ctx, source.GitRepository{URL: repoDir, Revision: "v1.0.0"})
	if err != nil {
		t.Fatalf("resolve tag: %v", err)
	}
	if tag.Revision != commit {
		t.Fatalf("tag revision = %q, want %q", tag.Revision, commit)
	}

	exact, err := cache.Resolve(ctx, source.GitRepository{URL: repoDir, Revision: commit})
	if err != nil {
		t.Fatalf("resolve exact commit: %v", err)
	}
	if exact.Revision != commit {
		t.Fatalf("exact revision = %q, want %q", exact.Revision, commit)
	}
}

func TestCacheSerializesConcurrentFetchesAndReusesCache(t *testing.T) {
	ctx := context.Background()
	repoDir, commit := createGitRepository(t)
	cache := NewCache(filepath.Join(t.TempDir(), "cache"))
	const workers = 8

	var wg sync.WaitGroup
	errs := make(chan error, workers)
	cacheDirs := make(chan string, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resolved, err := cache.Resolve(ctx, source.GitRepository{URL: repoDir, Revision: "main"})
			if err != nil {
				errs <- err
				return
			}
			if resolved.Revision != commit {
				errs <- errors.New("unexpected revision")
				return
			}
			cacheDirs <- resolved.CacheDir
		}()
	}
	wg.Wait()
	close(errs)
	close(cacheDirs)
	for err := range errs {
		t.Fatal(err)
	}
	var first string
	for dir := range cacheDirs {
		if first == "" {
			first = dir
			continue
		}
		if dir != first {
			t.Fatalf("cache dir = %q, want %q", dir, first)
		}
	}
}

func TestGCPreservesActiveCacheAndRemovesStaleDirectories(t *testing.T) {
	root := t.TempDir()
	cache := NewCache(root)
	active := filepath.Join(root, "active")
	stale := filepath.Join(root, "stale")
	for _, dir := range []string{active, stale} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	removed, err := cache.GC([]source.ResolvedSource{{CacheDir: active}}, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != stale {
		t.Fatalf("removed = %#v", removed)
	}
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("active cache removed: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale cache still exists or unexpected error: %v", err)
	}
}

func TestCacheLossCausesRefetchOnly(t *testing.T) {
	ctx := context.Background()
	repoDir, commit := createGitRepository(t)
	cache := NewCache(filepath.Join(t.TempDir(), "cache"))

	resolved, err := cache.Resolve(ctx, source.GitRepository{URL: repoDir, Revision: "main"})
	if err != nil {
		t.Fatalf("resolve before cache loss: %v", err)
	}
	if err := os.RemoveAll(resolved.CacheDir); err != nil {
		t.Fatalf("remove cache dir: %v", err)
	}
	afterLoss, err := cache.Resolve(ctx, source.GitRepository{URL: repoDir, Revision: "main"})
	if err != nil {
		t.Fatalf("resolve after cache loss: %v", err)
	}
	if afterLoss.Revision != commit {
		t.Fatalf("revision after cache loss = %q, want %q", afterLoss.Revision, commit)
	}
}

func TestSSHRepositoryWithoutKeyFailsAuthentication(t *testing.T) {
	_, err := NewCache(t.TempDir()).Resolve(context.Background(), source.GitRepository{URL: "ssh://git@example.com/acme/platform.git", Revision: "main"})
	var sourceErr *source.Error
	if !errors.As(err, &sourceErr) {
		t.Fatalf("error = %T, want *source.Error", err)
	}
	if sourceErr.Reason != source.FailureReasonAuthenticationFailure {
		t.Fatalf("reason = %q, want %q", sourceErr.Reason, source.FailureReasonAuthenticationFailure)
	}
}

func createGitRepository(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInitWithOptions(dir, &gogit.PlainInitOptions{InitOptions: gogit.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")}})
	if err != nil {
		t.Fatal(err)
	}
	commit := commitFiles(t, repo, dir, map[string]string{"app.yaml": "kind: ConfigMap\n"}, nil)
	if _, err := repo.CreateTag("v1.0.0", commit, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTag("v1.0.0-annotated", commit, &gogit.CreateTagOptions{Message: "release", Tagger: signature()}); err != nil {
		t.Fatal(err)
	}
	// The in-process file transport serves the Git directory itself.
	return filepath.Join(dir, ".git"), commit.String()
}

// commitFiles writes files and symlinks (name to target) and commits them.
func commitFiles(t *testing.T, repo *gogit.Repository, dir string, files, symlinks map[string]string) plumbing.Hash {
	t.Helper()
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := worktree.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	for name, target := range symlinks {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		if _, err := worktree.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	commit, err := worktree.Commit("commit", &gogit.CommitOptions{Author: signature()})
	if err != nil {
		t.Fatal(err)
	}
	return commit
}

func signature() *object.Signature {
	return &object.Signature{Name: "Solder Test", Email: "solder@example.com", When: time.Unix(1700000000, 0)}
}

func TestCacheResolvesAnnotatedTagsShortCommitsAndDefaultBranch(t *testing.T) {
	ctx := context.Background()
	repoDir, commit := createGitRepository(t)
	cache := NewCache(filepath.Join(t.TempDir(), "cache"))
	for _, revision := range []string{"v1.0.0-annotated", commit[:12], ""} {
		resolved, err := cache.Resolve(ctx, source.GitRepository{URL: repoDir, Revision: revision})
		if err != nil {
			t.Fatalf("resolve %q: %v", revision, err)
		}
		if resolved.Revision != commit {
			t.Fatalf("resolve %q = %q, want %q", revision, resolved.Revision, commit)
		}
		content, err := os.ReadFile(filepath.Join(resolved.CacheDir, "app.yaml"))
		if err != nil || string(content) != "kind: ConfigMap\n" {
			t.Fatalf("checked-out app.yaml = %q, %v", content, err)
		}
	}
}

func TestCacheRefusesSymlinksOutOfTheCheckout(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	repo, err := gogit.PlainInitWithOptions(dir, &gogit.PlainInitOptions{InitOptions: gogit.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")}})
	if err != nil {
		t.Fatal(err)
	}
	commitFiles(t, repo, dir, map[string]string{"app.yaml": "kind: ConfigMap\n"}, map[string]string{"inside": "app.yaml"})
	cache := NewCache(filepath.Join(t.TempDir(), "cache"))
	resolved, err := cache.Resolve(ctx, source.GitRepository{URL: filepath.Join(dir, ".git"), Revision: "main"})
	if err != nil {
		t.Fatalf("symlink inside the checkout: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(resolved.CacheDir, "inside")); err != nil || string(content) != "kind: ConfigMap\n" {
		t.Fatalf("inside symlink = %q, %v", content, err)
	}

	for _, target := range []string{"/var/run/secrets/kubernetes.io/serviceaccount/token", "../../../etc/passwd"} {
		commitFiles(t, repo, dir, nil, map[string]string{filepath.Base(target) + "-link": target})
		_, err := cache.Resolve(ctx, source.GitRepository{URL: filepath.Join(dir, ".git"), Revision: "main"})
		var sourceErr *source.Error
		if !errors.As(err, &sourceErr) || !strings.Contains(sourceErr.Message, "points outside the checkout") {
			t.Fatalf("symlink to %s: err = %v", target, err)
		}
	}
}

func TestSSHRepositoryWithoutKnownHostsFailsAuthentication(t *testing.T) {
	key := testSSHKey(t)
	_, err := NewCache(t.TempDir()).Resolve(context.Background(), source.GitRepository{
		URL: "git@example.com:acme/platform.git", Revision: "main", Auth: source.Credentials{SSHKey: key},
	})
	var sourceErr *source.Error
	if !errors.As(err, &sourceErr) || sourceErr.Reason != source.FailureReasonAuthenticationFailure || !strings.Contains(sourceErr.Message, "known_hosts") {
		t.Fatalf("err = %v", err)
	}
}

func TestLocalRepositoryURLsAreRejected(t *testing.T) {
	allowLocalRepositories = false
	defer func() { allowLocalRepositories = true }()
	for _, url := range []string{"/srv/repo", "file:///srv/repo", "./repo"} {
		_, err := NewCache(t.TempDir()).Resolve(context.Background(), source.GitRepository{URL: url, Revision: "main"})
		var sourceErr *source.Error
		if !errors.As(err, &sourceErr) || sourceErr.Reason != source.FailureReasonValidationFailure {
			t.Fatalf("%s: err = %v", url, err)
		}
	}
}

func testSSHKey(t *testing.T) string {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := gossh.MarshalPrivateKey(private, "")
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block))
}

// treeEntry is a raw Git tree entry: a blob when entries is nil, else a tree.
type treeEntry struct {
	name     string
	mode     filemode.FileMode
	contents string
	entries  []treeEntry
}

// commitTree stores the entries as given, in order and even when a name
// repeats, as a hostile remote could, and points main at the commit.
func commitTree(t *testing.T, entries []treeEntry) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInitWithOptions(dir, &gogit.PlainInitOptions{InitOptions: gogit.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")}})
	if err != nil {
		t.Fatal(err)
	}
	commit := &object.Commit{Author: *signature(), Committer: *signature(), Message: "commit", TreeHash: storeTree(t, repo, entries)}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), storeObject(t, repo, commit))); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, ".git")
}

func storeTree(t *testing.T, repo *gogit.Repository, entries []treeEntry) plumbing.Hash {
	t.Helper()
	tree := &object.Tree{}
	for _, entry := range entries {
		if entry.entries != nil {
			tree.Entries = append(tree.Entries, object.TreeEntry{Name: entry.name, Mode: filemode.Dir, Hash: storeTree(t, repo, entry.entries)})
			continue
		}
		blob := repo.Storer.NewEncodedObject()
		blob.SetType(plumbing.BlobObject)
		writer, err := blob.Writer()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(entry.contents)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		hash, err := repo.Storer.SetEncodedObject(blob)
		if err != nil {
			t.Fatal(err)
		}
		tree.Entries = append(tree.Entries, object.TreeEntry{Name: entry.name, Mode: entry.mode, Hash: hash})
	}
	return storeObject(t, repo, tree)
}

func storeObject(t *testing.T, repo *gogit.Repository, obj interface {
	Encode(plumbing.EncodedObject) error
}) plumbing.Hash {
	t.Helper()
	encoded := repo.Storer.NewEncodedObject()
	if err := obj.Encode(encoded); err != nil {
		t.Fatal(err)
	}
	hash, err := repo.Storer.SetEncodedObject(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func link(name, target string) treeEntry {
	return treeEntry{name: name, mode: filemode.Symlink, contents: target}
}

func TestCacheRefusesSymlinkChainsOutOfTheCheckout(t *testing.T) {
	regular := treeEntry{name: "app.yaml", mode: filemode.Regular, contents: "kind: ConfigMap\n"}
	for name, entries := range map[string][]treeEntry{
		// Every link is inside on its own, but on disk s is the parent
		// directory, so writing the marker would follow the chain out.
		"marker through a chain": {link(".solder-checkout", "s/x"), regular, link("s", "t/.."), link("t", ".")},
		"climbing chain":         {regular, link("s0", "."), link("s1", "s0/.."), link("s2", "s1/.."), link("s3", "s2/..")},
		// A tree may repeat a name, so a file can land below an earlier link.
		"file below a link": {link("a", "."), link("s", "a/.."), {name: "s", entries: []treeEntry{{name: "x", mode: filemode.Regular, contents: "pwned"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			repoDir := commitTree(t, entries)
			cache := NewCache(filepath.Join(t.TempDir(), "cache"))
			_, err := cache.Resolve(context.Background(), source.GitRepository{URL: repoDir, Revision: "main"})
			var sourceErr *source.Error
			if !errors.As(err, &sourceErr) || sourceErr.Reason != source.FailureReasonValidationFailure {
				t.Fatalf("err = %v", err)
			}
			if written, _ := filepath.Glob(filepath.Join(cache.Root, "*", "worktrees", "*")); len(written) != 0 {
				t.Fatalf("written outside the checkout: %v", written)
			}
		})
	}
}

func TestCacheKeepsSymlinkChainsAndDanglingLinksInside(t *testing.T) {
	repoDir := commitTree(t, []treeEntry{
		link("a", "b"),
		link("b", "dir/../dir"),
		link("dangling", "missing/x"),
		{name: "dir", entries: []treeEntry{{name: "app.yaml", mode: filemode.Regular, contents: "kind: ConfigMap\n"}}},
	})
	resolved, err := NewCache(filepath.Join(t.TempDir(), "cache")).Resolve(context.Background(), source.GitRepository{URL: repoDir, Revision: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(filepath.Join(resolved.CacheDir, "a", "app.yaml")); err != nil || string(content) != "kind: ConfigMap\n" {
		t.Fatalf("a/app.yaml = %q, %v", content, err)
	}
}

func TestCacheDoesNotGuessADetachedRemoteHEAD(t *testing.T) {
	dir := t.TempDir()
	// master is also the local cache's own HEAD, so falling back to it
	// would quietly deploy the wrong commit.
	repo, err := gogit.PlainInitWithOptions(dir, &gogit.PlainInitOptions{InitOptions: gogit.InitOptions{DefaultBranch: plumbing.Master}})
	if err != nil {
		t.Fatal(err)
	}
	first := commitFiles(t, repo, dir, map[string]string{"app.yaml": "kind: ConfigMap\n"}, nil)
	commitFiles(t, repo, dir, map[string]string{"app.yaml": "kind: Secret\n"}, nil)
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, first)); err != nil {
		t.Fatal(err)
	}
	resolved, err := NewCache(filepath.Join(t.TempDir(), "cache")).Resolve(context.Background(), source.GitRepository{URL: filepath.Join(dir, ".git")})
	if err == nil && resolved.Revision != first.String() {
		t.Fatalf("revision = %s, want the detached HEAD %s or an error", resolved.Revision, first)
	}
}
