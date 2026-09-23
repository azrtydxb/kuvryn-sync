// Package imageupdate rewrites Flux-compatible image policy markers in a Git
// repository and pushes the result.
package imageupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/util"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/azrtydxb/solder/internal/source"
	gitsource "github.com/azrtydxb/solder/internal/source/git"
)

// Image is what an ImagePolicy currently selects.
type Image struct {
	// Name is the image repository, e.g. ghcr.io/acme/api.
	Name string
	// Tag is the selected tag.
	Tag string
	// Full is the immutable reference name:tag@digest.
	Full string
}

// Change is one rewritten marker.
type Change struct {
	File   string
	Policy string
	Old    string
	New    string
}

// marker matches `key: value # {"$imagepolicy": "ns:name[:tag|:name]"}`.
var marker = regexp.MustCompile(`^(\s*(?:-\s+)?[^#\n]*?:\s+)(["']?)([^\s"'#]+)(["']?)(\s+#\s*\{\s*"\$imagepolicy"\s*:\s*"([^"]+)"\s*\}.*)$`)

// Rewrite updates markers for policies in namespace; markers naming another
// namespace are left alone, so one tenant cannot pin another's images.
func Rewrite(file string, data []byte, namespace string, images map[string]Image) ([]byte, []Change) {
	lines := strings.Split(string(data), "\n")
	changes := []Change{}
	for i, line := range lines {
		groups := marker.FindStringSubmatch(line)
		if groups == nil {
			continue
		}
		ref := strings.Split(groups[6], ":")
		if len(ref) < 2 || ref[0] != namespace {
			continue
		}
		image, ok := images[ref[0]+":"+ref[1]]
		if !ok {
			continue
		}
		value := image.Full
		if len(ref) == 3 {
			switch ref[2] {
			case "tag":
				value = image.Tag
			case "name":
				value = image.Name
			default:
				continue
			}
		}
		if value == "" || value == groups[3] {
			continue
		}
		lines[i] = groups[1] + groups[2] + value + groups[4] + groups[5]
		changes = append(changes, Change{File: file, Policy: ref[0] + ":" + ref[1], Old: groups[3], New: value})
	}
	return []byte(strings.Join(lines, "\n")), changes
}

// Request is one write-back.
type Request struct {
	URL       string
	Branch    string
	Path      string
	Auth      source.Credentials
	Author    object.Signature
	Namespace string
	Images    map[string]Image
}

// Updater commits image changes and pushes them.
type Updater struct {
	// AllowLocal permits filesystem repository URLs; only tests set it.
	AllowLocal bool
	// BeforePush runs between commit and push; tests use it to race a push.
	BeforePush func()
}

const attempts = 3

// Update clones the branch, rewrites markers, and pushes a commit when
// anything changed, retrying when the branch moved in the meantime. It
// returns the pushed commit, or "" when nothing needed to change.
func (u *Updater) Update(ctx context.Context, req Request) (string, []Change, error) {
	if !gitsource.RemoteURL(req.URL) && !u.AllowLocal {
		return "", nil, fmt.Errorf("git repository URL must use https, http, ssh, or git")
	}
	auth, err := gitsource.AuthMethod(source.GitRepository{URL: req.URL, Auth: req.Auth})
	if err != nil {
		return "", nil, err
	}
	for attempt := 1; ; attempt++ {
		commit, changes, err := u.attempt(ctx, req, auth)
		if err == nil || !branchMoved(err) || attempt == attempts {
			return commit, changes, err
		}
	}
}

// branchMoved reports a push rejected because the branch gained commits.
// go-git returns its own check as an unwrapped string and relays server
// rejections as text, so the message is matched as well as the sentinel.
func branchMoved(err error) bool {
	if errors.Is(err, gogit.ErrNonFastForwardUpdate) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "non-fast-forward") || strings.Contains(msg, "fetch first")
}

func (u *Updater) attempt(ctx context.Context, req Request, auth transport.AuthMethod) (string, []Change, error) {
	fs := memfs.New()
	options := &gogit.CloneOptions{URL: req.URL, Auth: auth, ReferenceName: plumbing.NewBranchReferenceName(req.Branch), SingleBranch: true}
	// debt: the whole branch history is cloned into memory on each write-back;
	// revisit if repositories become large enough for this to matter.
	repo, err := gogit.CloneContext(ctx, memory.NewStorage(), fs, options)
	if err != nil {
		return "", nil, fmt.Errorf("clone %s: %w", req.Branch, err)
	}
	changes, err := rewriteTree(fs, cleanPath(req.Path), req.Namespace, req.Images)
	if err != nil || len(changes) == 0 {
		return "", nil, err
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return "", nil, err
	}
	for _, change := range changes {
		if _, err := worktree.Add(change.File); err != nil {
			return "", nil, err
		}
	}
	author := req.Author
	author.When = time.Now()
	hash, err := worktree.Commit(message(changes), &gogit.CommitOptions{Author: &author})
	if err != nil {
		return "", nil, err
	}
	if u.BeforePush != nil {
		u.BeforePush()
	}
	refspec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", req.Branch, req.Branch))
	if err := repo.PushContext(ctx, &gogit.PushOptions{Auth: auth, RefSpecs: []config.RefSpec{refspec}}); err != nil {
		return "", nil, fmt.Errorf("push %s: %w", req.Branch, err)
	}
	return hash.String(), changes, nil
}

func rewriteTree(fs billy.Filesystem, root, namespace string, images map[string]Image) ([]Change, error) {
	changes := []Change{}
	err := util.Walk(fs, root, func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !(strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
			return nil
		}
		file, err := fs.Open(name)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(file)
		_ = file.Close()
		if err != nil {
			return err
		}
		updated, fileChanges := Rewrite(strings.TrimPrefix(name, "/"), data, namespace, images)
		if len(fileChanges) == 0 {
			return nil
		}
		changes = append(changes, fileChanges...)
		return util.WriteFile(fs, name, updated, info.Mode())
	})
	return changes, err
}

func cleanPath(p string) string {
	return path.Clean("/" + strings.TrimSpace(p))
}

func message(changes []Change) string {
	lines := make([]string, 0, len(changes))
	for _, change := range changes {
		lines = append(lines, fmt.Sprintf("- %s (%s): %s -> %s", change.Policy, change.File, change.Old, change.New))
	}
	sort.Strings(lines)
	return "Update images from ImagePolicies\n\n" + strings.Join(lines, "\n") + "\n"
}
