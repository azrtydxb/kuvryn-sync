// Package kustomize renders Kustomize overlays in process.
package kustomize

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/azrtydxb/solder/internal/renderer"
	yamlrenderer "github.com/azrtydxb/solder/internal/renderer/yaml"
)

// Renderer builds a kustomization with the kustomize library, the same code
// `kubectl kustomize` uses.
type Renderer struct{}

// Render builds input.Path against an in-memory copy of the workspace, so
// bases and resources can only come from the checked-out repository.
func (Renderer) Render(_ context.Context, input renderer.Input) ([]unstructured.Unstructured, error) {
	dir, err := renderer.Dir(input)
	if err != nil {
		return nil, err
	}
	workspace, err := inMemory(input.Workspace, input.Decrypt)
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
// that stay inside it and decrypting SOPS files, into an in-memory
// filesystem rooted at "/".
func inMemory(workspace string, decrypt func(string, []byte) ([]byte, error)) (filesys.FileSystem, error) {
	if err := renderer.Contained(workspace, workspace); err != nil {
		return nil, err
	}
	memory := filesys.MakeFsInMemory()
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
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Decrypt before Kustomize transforms anything: the SOPS MAC covers
		// the whole document, including names a prefix would change.
		if decrypt != nil {
			if data, err = decrypt(rel, data); err != nil {
				return err
			}
		}
		return memory.WriteFile(target, data)
	})
	if err != nil {
		return nil, fmt.Errorf("read workspace: %w", err)
	}
	return memory, nil
}

var _ renderer.Renderer = Renderer{}
