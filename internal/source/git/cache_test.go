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
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/azrtydxb/solder/internal/source"
)

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
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "solder@example.com")
	runGit(t, dir, "config", "user.name", "Solder Test")
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte("kind: ConfigMap\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	runGit(t, dir, "add", "app.yaml")
	runGit(t, dir, "commit", "-m", "initial")
	runGit(t, dir, "tag", "v1.0.0")
	commit := runGit(t, dir, "rev-parse", "HEAD")
	return dir, commit[:len(commit)-1]
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
