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
	"regexp"
	"strings"

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
	// The release name comes from the Application spec. Unchecked and placed
	// before any "--", a name like "--post-renderer=<path>" was read by Helm
	// as a flag, and --post-renderer runs a program: argument injection into
	// the controller. It must be a valid Helm release name, and the
	// positional arguments go after "--" so nothing there is read as a flag.
	if len(releaseName) > 53 || !releaseNameRE.MatchString(releaseName) {
		return nil, fmt.Errorf("helm release name %q is not valid: lowercase letters, digits, '-' and '.', at most 53 characters", releaseName)
	}
	args := []string{"template"}
	for _, valuesFile := range input.ValuesFiles {
		if filepath.IsAbs(valuesFile) {
			return nil, fmt.Errorf("helm values file must be relative")
		}
		values, err := within(input.Workspace, filepath.Join(path, filepath.Clean(valuesFile)))
		if err != nil {
			return nil, fmt.Errorf("helm values file %q: %w", valuesFile, err)
		}
		args = append(args, "-f", values)
	}
	args = append(args, "--", releaseName, path)
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

// within resolves p, following symbolic links, and refuses it unless it stays
// inside workspace.
//
// Values files come from the Application spec and the repository, both
// untrusted. Only absolute paths used to be refused, so "../../x" walked out
// of the checkout, and a values file that is itself a link pointed anywhere;
// either handed Helm a file from the controller's filesystem, such as its
// service account token.
func within(workspace, p string) (string, error) {
	root, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("must stay inside the workspace")
	}
	// The resolved path, so Helm reads exactly the file that was checked.
	return real, nil
}

// releaseNameRE is Helm's own rule for a release name.
var releaseNameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

func command(ctx context.Context, binary string, args ...string) ([]byte, error) {
	// binary is helm or kustomize (tests substitute a fake); args is an argv,
	// never a shell string, and the one user-supplied value, the release name,
	// is validated and placed after "--".
	cmd := osexec.CommandContext(ctx, binary, args...) // #nosec G204 -- nosemgrep: dangerous-exec-command -- fixed binary, argv without a shell, validated release name after --
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
