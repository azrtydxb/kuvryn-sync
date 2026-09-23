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

package yaml

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/azrtydxb/solder/internal/renderer"
)

// Renderer decodes plain Kubernetes YAML manifests from a source path.
type Renderer struct{}

// Render decodes all YAML documents under input.Path in deterministic order.
func (Renderer) Render(ctx context.Context, input renderer.Input) ([]unstructured.Unstructured, error) {
	root, err := safePath(input.Workspace, input.Path)
	if err != nil {
		return nil, err
	}
	if root, err = resolvedInside(input.Workspace, root); err != nil {
		return nil, err
	}
	files, err := manifestFiles(root)
	if err != nil {
		return nil, err
	}
	objects := make([]unstructured.Unstructured, 0)
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read YAML manifest %s: %w", file, err)
		}
		decoded, err := Decode(data)
		if err != nil {
			return nil, fmt.Errorf("decode YAML manifest %s: %w", file, err)
		}
		objects = append(objects, decoded...)
	}
	return objects, nil
}

// Decode decodes multi-document Kubernetes YAML into unstructured objects.
func Decode(data []byte) ([]unstructured.Unstructured, error) {
	decoder := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	objects := make([]unstructured.Unstructured, 0)
	for {
		obj := map[string]any{}
		if err := decoder.Decode(&obj); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if len(obj) == 0 {
			continue
		}
		objects = append(objects, unstructured.Unstructured{Object: obj})
	}
	return objects, nil
}

// resolvedInside resolves p through symbolic links and refuses it unless it
// stays inside workspace: a render path that is a link to a directory or file
// outside the checkout passed safePath's lexical check.
func resolvedInside(workspace, p string) (string, error) {
	base, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", fmt.Errorf("inspect render path: %w", err)
	}
	rel, err := filepath.Rel(base, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("render path must stay inside workspace")
	}
	return real, nil
}

func manifestFiles(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect render path: %w", err)
	}
	if !info.IsDir() {
		if isYAML(root) {
			return []string{root}, nil
		}
		return nil, fmt.Errorf("render path %s is not a YAML file", root)
	}
	files := make([]string, 0)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		// A linked manifest can point anywhere on the controller's
		// filesystem, and whatever it points at would be decoded and
		// applied. WalkDir lists links without following them; reading one
		// would follow it, so links are skipped.
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if isYAML(path) {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func safePath(workspace, rel string) (string, error) {
	if workspace == "" {
		return "", fmt.Errorf("workspace is required")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("render path must be relative")
	}
	clean := filepath.Clean(rel)
	if clean == "." {
		clean = ""
	}
	if strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("render path must stay inside workspace")
	}
	return filepath.Join(workspace, clean), nil
}

func isYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

var _ renderer.Renderer = Renderer{}
