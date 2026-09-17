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

package exec

import (
	"context"
	"fmt"
	osexec "os/exec"
	"path/filepath"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/azrtydxb/solder/internal/renderer"
	yamlrenderer "github.com/azrtydxb/solder/internal/renderer/yaml"
)

// KustomizeRenderer renders desired state with kustomize build.
type KustomizeRenderer struct {
	Binary string
}

// HelmRenderer renders desired state with helm template.
type HelmRenderer struct {
	Binary string
}

func (r KustomizeRenderer) Render(ctx context.Context, input renderer.Input) ([]unstructured.Unstructured, error) {
	binary := r.Binary
	if binary == "" {
		binary = "kustomize"
	}
	path, err := renderPath(input)
	if err != nil {
		return nil, err
	}
	out, err := command(ctx, binary, "build", path)
	if err != nil {
		return nil, fmt.Errorf("kustomize render failed: %w", err)
	}
	return yamlrenderer.Decode(out)
}

func (r HelmRenderer) Render(ctx context.Context, input renderer.Input) ([]unstructured.Unstructured, error) {
	binary := r.Binary
	if binary == "" {
		binary = "helm"
	}
	path, err := renderPath(input)
	if err != nil {
		return nil, err
	}
	releaseName := input.ReleaseName
	if releaseName == "" {
		releaseName = "solder"
	}
	args := []string{"template", releaseName, path}
	for _, valuesFile := range input.ValuesFiles {
		if filepath.IsAbs(valuesFile) {
			return nil, fmt.Errorf("Helm values file must be relative")
		}
		args = append(args, "-f", filepath.Join(path, filepath.Clean(valuesFile)))
	}
	out, err := command(ctx, binary, args...)
	if err != nil {
		return nil, fmt.Errorf("helm render failed: %w", err)
	}
	return yamlrenderer.Decode(out)
}

func renderPath(input renderer.Input) (string, error) {
	if input.Workspace == "" {
		return "", fmt.Errorf("workspace is required")
	}
	if filepath.IsAbs(input.Path) {
		return "", fmt.Errorf("render path must be relative")
	}
	clean := filepath.Clean(input.Path)
	if clean == "." {
		clean = ""
	}
	if clean == ".." || filepath.IsAbs(clean) || len(clean) >= 3 && clean[:3] == "../" {
		return "", fmt.Errorf("render path must stay inside workspace")
	}
	return filepath.Join(input.Workspace, clean), nil
}

func command(ctx context.Context, binary string, args ...string) ([]byte, error) {
	cmd := osexec.CommandContext(ctx, binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 512 {
			out = out[:512]
		}
		return nil, fmt.Errorf("%s: %w", out, err)
	}
	return out, nil
}

var _ renderer.Renderer = KustomizeRenderer{}
var _ renderer.Renderer = HelmRenderer{}
