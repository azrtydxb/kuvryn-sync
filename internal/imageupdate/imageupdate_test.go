package imageupdate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
)

func init() {
	client.InstallProtocol("file", server.DefaultServer)
}

var images = map[string]Image{
	"payments:api": {Name: "ghcr.io/acme/api", Tag: "1.1.0", Full: "ghcr.io/acme/api:1.1.0@sha256:2222"},
	"search:api":   {Name: "ghcr.io/acme/search", Tag: "9.9.9", Full: "ghcr.io/acme/search:9.9.9@sha256:9999"},
}

func TestRewriteMarkers(t *testing.T) {
	in := `spec:
  containers:
    - name: api
      image: ghcr.io/acme/api:1.0.0 # {"$imagepolicy": "payments:api"}
    - image: "ghcr.io/acme/api:1.0.0" # {"$imagepolicy": "payments:api"}
  tag: 1.0.0 # {"$imagepolicy": "payments:api:tag"}
  repo: old/name # {"$imagepolicy": "payments:api:name"}
  other: ghcr.io/evil/x:1 # {"$imagepolicy": "search:api"}
  plain: ghcr.io/acme/api:1.0.0
`
	out, changes := Rewrite("deploy.yaml", []byte(in), "payments", images)
	got := string(out)
	for _, want := range []string{
		`      image: ghcr.io/acme/api:1.1.0@sha256:2222 # {"$imagepolicy": "payments:api"}`,
		`    - image: "ghcr.io/acme/api:1.1.0@sha256:2222" # {"$imagepolicy": "payments:api"}`,
		`  tag: 1.1.0 # {"$imagepolicy": "payments:api:tag"}`,
		`  repo: ghcr.io/acme/api # {"$imagepolicy": "payments:api:name"}`,
		`  other: ghcr.io/evil/x:1 # {"$imagepolicy": "search:api"}`,
		`  plain: ghcr.io/acme/api:1.0.0`,
	} {
		if !strings.Contains(got, want+"\n") {
			t.Errorf("missing line %q in:\n%s", want, got)
		}
	}
	if len(changes) != 4 {
		t.Fatalf("changes = %#v", changes)
	}
	if again, more := Rewrite("deploy.yaml", out, "payments", images); len(more) != 0 || string(again) != got {
		t.Fatalf("rewrite is not idempotent: %#v", more)
	}
}

func TestRewriteSkipsMarkersWithExtraParts(t *testing.T) {
	in := "tag: 1.0.0 # {\"$imagepolicy\": \"payments:api:tag:extra\"}\n"
	out, changes := Rewrite("deploy.yaml", []byte(in), "payments", images)
	if len(changes) != 0 || string(out) != in {
		t.Fatalf("a marker with four parts was rewritten: %q, %#v", out, changes)
	}
}

// origin creates a bare repository with one commit on main and returns its path.
func origin(t *testing.T) string {
	t.Helper()
	seed := t.TempDir()
	repo, err := gogit.PlainInitWithOptions(seed, &gogit.PlainInitOptions{InitOptions: gogit.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")}})
	if err != nil {
		t.Fatal(err)
	}
	commitFile(t, repo, seed, "apps/api/deploy.yaml", "image: ghcr.io/acme/api:1.0.0 # {\"$imagepolicy\": \"payments:api\"}\n")
	bare := filepath.Join(t.TempDir(), "origin.git")
	if _, err := gogit.PlainClone(bare, true, &gogit.CloneOptions{URL: filepath.Join(seed, ".git")}); err != nil {
		t.Fatal(err)
	}
	return bare
}

func commitFile(t *testing.T, repo *gogit.Repository, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add(name); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("seed", &gogit.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()}}); err != nil {
		t.Fatal(err)
	}
}

func headFile(t *testing.T, bare, name string) (string, *object.Commit) {
	t.Helper()
	repo, err := gogit.PlainOpen(bare)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := repo.Reference(plumbing.NewBranchReferenceName("main"), true)
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		t.Fatal(err)
	}
	file, err := commit.File(name)
	if err != nil {
		t.Fatal(err)
	}
	content, err := file.Contents()
	if err != nil {
		t.Fatal(err)
	}
	return content, commit
}

func request(url string) Request {
	return Request{URL: url, Branch: "main", Path: "apps", Namespace: "payments", Images: images, Author: object.Signature{Name: "Kuvryn Sync", Email: "kuvryn-sync@localhost"}}
}

func TestUpdateCommitsAndPushesOnce(t *testing.T) {
	bare := origin(t)
	updater := &Updater{AllowLocal: true}
	commit, changes, err := updater.Update(context.Background(), request(bare))
	if err != nil || commit == "" || len(changes) != 1 {
		t.Fatalf("update = %q, %v, %v", commit, changes, err)
	}
	content, head := headFile(t, bare, "apps/api/deploy.yaml")
	if head.Hash.String() != commit || !strings.Contains(content, "ghcr.io/acme/api:1.1.0@sha256:2222") {
		t.Fatalf("origin head %s content:\n%s", head.Hash, content)
	}
	if !strings.Contains(head.Message, "payments:api (apps/api/deploy.yaml): ghcr.io/acme/api:1.0.0 -> ghcr.io/acme/api:1.1.0@sha256:2222") || head.Author.Name != "Kuvryn Sync" {
		t.Fatalf("commit = %q by %q", head.Message, head.Author.Name)
	}

	again, _, err := updater.Update(context.Background(), request(bare))
	if err != nil || again != "" {
		t.Fatalf("second update committed %q, %v", again, err)
	}
}

func TestUpdateRetriesWhenTheBranchMoves(t *testing.T) {
	bare := origin(t)
	raced := false
	updater := &Updater{AllowLocal: true, BeforePush: func() {
		if raced {
			return
		}
		raced = true
		other := t.TempDir()
		repo, err := gogit.PlainClone(other, false, &gogit.CloneOptions{URL: bare})
		if err != nil {
			t.Fatal(err)
		}
		commitFile(t, repo, other, "README.md", "concurrent change\n")
		if err := repo.Push(&gogit.PushOptions{}); err != nil {
			t.Fatal(err)
		}
	}}
	commit, _, err := updater.Update(context.Background(), request(bare))
	if err != nil || commit == "" {
		t.Fatalf("update = %q, %v", commit, err)
	}
	if readme, _ := headFile(t, bare, "README.md"); readme != "concurrent change\n" {
		t.Fatal("the concurrent commit was lost")
	}
	if content, _ := headFile(t, bare, "apps/api/deploy.yaml"); !strings.Contains(content, "1.1.0@sha256:2222") {
		t.Fatalf("image not updated after retry:\n%s", content)
	}
}
