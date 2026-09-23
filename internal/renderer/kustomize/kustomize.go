// Package kustomize renders Kustomize overlays in process.
package kustomize

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/azrtydxb/solder/internal/renderer"
	yamlrenderer "github.com/azrtydxb/solder/internal/renderer/yaml"
)

// Renderer builds a kustomization with the kustomize library, the same code
// `kubectl kustomize` uses.
type Renderer struct{}

// Render builds input.Path against an in-memory copy of the workspace.
// kustomize/api would fetch http(s) URLs and clone Git remotes named in a
// kustomization, with no timeout or size limit, so every kustomization it
// reads is checked first and remote references are refused; bases and
// resources then come only from the checked-out repository.
func (Renderer) Render(ctx context.Context, input renderer.Input) ([]unstructured.Unstructured, error) {
	dir, err := renderer.Dir(input)
	if err != nil {
		return nil, err
	}
	workspace, err := inMemory(ctx, input.Workspace, input.Decrypt)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(input.Workspace, dir)
	if err != nil {
		return nil, err
	}
	// debt: the whole workspace is copied into memory on every render; revisit
	// if repositories grow large enough for this to matter.
	resources, err := krusty.MakeKustomizer(krusty.MakeDefaultOptions()).Run(workspace, filepath.Join("/", rel))
	if workspace.refused != nil {
		return nil, workspace.refused
	}
	if err != nil {
		return nil, fmt.Errorf("kustomize render failed: %w", err)
	}
	out, err := resources.AsYaml()
	if err != nil {
		return nil, fmt.Errorf("kustomize render failed: %w", err)
	}
	return yamlrenderer.Decode(out)
}

// inMemory copies the workspace's regular files, following only symlinks
// that stay inside it, into an in-memory filesystem rooted at "/". Links
// that dangle are skipped, and links out of the workspace fail only when
// kustomize reads them.
func inMemory(ctx context.Context, workspace string, decrypt func(string, []byte) ([]byte, error)) (*guarded, error) {
	memory := &guarded{FileSystem: filesys.MakeFsInMemory(), ctx: ctx, decrypt: decrypt, outside: map[string]bool{}}
	err := filepath.WalkDir(workspace, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		target := filepath.Join("/", rel)
		if entry.IsDir() {
			return memory.MkdirAll(target)
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			// A link that does not resolve has nothing kustomize could read.
			inside, resolveErr := renderer.Within(workspace, path)
			if resolveErr != nil {
				return nil
			}
			if !inside {
				memory.outside[target] = true
				return memory.WriteFile(target, nil)
			}
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return memory.WriteFile(target, data)
	})
	if err != nil {
		return nil, fmt.Errorf("read workspace: %w", err)
	}
	return memory, nil
}

// guarded checks each file as kustomize reads it: it decrypts SOPS files,
// refuses links out of the workspace and kustomizations with remote
// references, and stops once ctx is done. Only files a render uses are
// checked, so an unrelated file elsewhere in a monorepo cannot break it.
type guarded struct {
	filesys.FileSystem
	ctx     context.Context
	decrypt func(string, []byte) ([]byte, error)
	outside map[string]bool
	// refused is the first refusal. Kustomize reports an unreadable
	// kustomization as missing, so the render returns this instead.
	refused error
}

func (g *guarded) ReadFile(path string) ([]byte, error) {
	if g.refused != nil {
		return nil, g.refused
	}
	data, err := g.FileSystem.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if data, err = g.check(path, data); err != nil {
		g.refused = err
		return nil, err
	}
	return data, nil
}

func (g *guarded) check(path string, data []byte) ([]byte, error) {
	if err := g.ctx.Err(); err != nil {
		return nil, err
	}
	rel := strings.TrimPrefix(filepath.Clean(path), "/")
	if g.outside[filepath.Clean(path)] {
		return nil, fmt.Errorf("symlink %s points outside the workspace", rel)
	}
	// Decrypt before Kustomize transforms anything: the SOPS MAC covers
	// the whole document, including names a prefix would change.
	if g.decrypt != nil {
		var err error
		if data, err = g.decrypt(rel, data); err != nil {
			return nil, err
		}
	}
	if kustomizationFile(path) {
		if err := refuseRemote(rel, data); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func kustomizationFile(path string) bool {
	switch filepath.Base(path) {
	case "kustomization.yaml", "kustomization.yml", "Kustomization":
		return true
	}
	return false
}

// refuseRemote fails if the kustomization names a URL or a Git remote in any
// field kustomize/api loads through its loader.
func refuseRemote(path string, data []byte) error {
	var doc map[string]any
	if err := sigsyaml.Unmarshal(data, &doc); err != nil {
		// Kustomize reports the malformed file itself.
		return nil
	}
	for _, ref := range loadedReferences(doc) {
		if remoteReference(ref) {
			return fmt.Errorf("kustomization %s refers to remote %s; only files in the repository can be used", path, ref)
		}
	}
	return nil
}

func loadedReferences(doc map[string]any) []string {
	var refs []string
	addAll := func(value any) {
		list, _ := value.([]any)
		for _, item := range list {
			// Multi-line entries are inline patches or configs, not paths.
			if ref, ok := item.(string); ok && !strings.Contains(ref, "\n") {
				refs = append(refs, ref)
			}
		}
	}
	add := func(value any) {
		if ref, ok := value.(string); ok {
			refs = append(refs, ref)
		}
	}
	each := func(value any, visit func(map[string]any)) {
		list, _ := value.([]any)
		for _, item := range list {
			if entry, ok := item.(map[string]any); ok {
				visit(entry)
			}
		}
	}
	for _, key := range []string{"resources", "components", "bases", "crds", "configurations", "generators", "transformers", "validators", "patchesStrategicMerge"} {
		addAll(doc[key])
	}
	for _, key := range []string{"patches", "patchesJson6902", "replacements"} {
		each(doc[key], func(entry map[string]any) { add(entry["path"]) })
	}
	for _, key := range []string{"configMapGenerator", "secretGenerator"} {
		each(doc[key], func(entry map[string]any) {
			addAll(entry["files"])
			addAll(entry["envs"])
			add(entry["env"])
		})
	}
	if openapi, ok := doc["openapi"].(map[string]any); ok {
		add(openapi["path"])
	}
	return refs
}

// remoteReference reports whether kustomize/api would fetch ref over the
// network: a URL, or a Git remote in any form its repo spec parser accepts.
func remoteReference(ref string) bool {
	lower := strings.ToLower(strings.TrimSpace(ref))
	for _, marker := range []string{"://", "?ref=", "?version=", "?timeout=", "?submodules="} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, prefix := range []string{"git::", "github.com/", "github.com:"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	// SCP-like Git syntax: user@host:path.
	at := strings.Index(lower, "@")
	return at > 0 && strings.Contains(lower[at:], ":")
}

var _ renderer.Renderer = Renderer{}
