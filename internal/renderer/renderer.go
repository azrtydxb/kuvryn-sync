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

package renderer

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Input describes a checked-out desired-state workspace and render options.
type Input struct {
	Workspace   string
	Path        string
	ReleaseName string
	ValuesFiles []string
	// Namespace is the release namespace for renderers that template it.
	Namespace string
	// Decrypt, when set, is applied to every file a renderer reads from the
	// workspace; it returns SOPS-encrypted files decrypted and others as is.
	Decrypt func(path string, data []byte) ([]byte, error)
}

// Renderer turns a desired-state source path into Kubernetes objects.
type Renderer interface {
	Render(ctx context.Context, input Input) ([]unstructured.Unstructured, error)
}

// Dir returns the directory of input.Path inside input.Workspace, rejecting
// paths that are absolute or climb out of the workspace.
func Dir(input Input) (string, error) {
	if input.Workspace == "" {
		return "", fmt.Errorf("workspace is required")
	}
	if filepath.IsAbs(input.Path) {
		return "", fmt.Errorf("render path must be relative")
	}
	clean := filepath.Clean(input.Path)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("render path must stay inside workspace")
	}
	return filepath.Join(input.Workspace, clean), nil
}

// Within reports whether path, with symlinks resolved, lies inside root.
func Within(root, path string) (bool, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false, err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil {
		return false, nil
	}
	return rel != ".." && !strings.HasPrefix(rel, "../"), nil
}

// Contained returns an error if any symlink under dir resolves outside root,
// so renderers never read files beyond the checked-out workspace.
func Contained(root, dir string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		inside, err := Within(root, path)
		if err != nil {
			return fmt.Errorf("resolve symlink %s: %w", relative(root, path), err)
		}
		if !inside {
			return fmt.Errorf("symlink %s points outside the workspace", relative(root, path))
		}
		return nil
	})
}

func relative(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return rel
	}
	return path
}
