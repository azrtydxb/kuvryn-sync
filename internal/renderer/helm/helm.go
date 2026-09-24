// Package helm renders Helm charts in process with the Helm SDK.
package helm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart"
	chartloader "helm.sh/helm/v4/pkg/chart/loader"
	valuesloader "helm.sh/helm/v4/pkg/chart/v2/loader"
	release "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/azrtydxb/solder/internal/renderer"
	yamlrenderer "github.com/azrtydxb/solder/internal/renderer/yaml"
)

const defaultReleaseName = "solder"

// ReleaseName returns the Helm release name rendering uses: name as given in
// the Application's spec, or the default "solder" when name is empty.
func ReleaseName(name string) string {
	if name == "" {
		return defaultReleaseName
	}
	return name
}

// Renderer renders a chart like `helm template`, without a helm binary and
// without contacting the cluster or any chart or values URL.
type Renderer struct{}

// Render templates the chart at input.Path with the given values files.
func (Renderer) Render(ctx context.Context, input renderer.Input) ([]unstructured.Unstructured, error) {
	dir, err := renderer.Dir(input)
	if err != nil {
		return nil, err
	}
	// A chart pulled from a repository reads nothing from Git unless values
	// files are given, so source.path need not exist then.
	if input.ChartPath == "" || len(input.ValuesFiles) > 0 {
		if err := renderer.Contained(input.Workspace, dir); err != nil {
			return nil, err
		}
	}
	values, err := loadValues(input.Workspace, dir, input.ValuesFiles, input.Decrypt)
	if err != nil {
		return nil, err
	}
	values = valuesloader.MergeMaps(values, input.Values)
	chartPath := dir
	if input.ChartPath != "" {
		chartPath = input.ChartPath
	}
	chrt, err := chartloader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("load Helm chart: %w", err)
	}
	accessor, err := chart.NewAccessor(chrt)
	if err != nil {
		return nil, fmt.Errorf("load Helm chart: %w", err)
	}
	if deps := accessor.MetaDependencies(); len(deps) > 0 {
		if err := action.CheckDependencies(chrt, deps); err != nil {
			return nil, fmt.Errorf("chart dependencies are missing; vendor them into charts/ with `helm dependency build`: %w", err)
		}
	}

	install := action.NewInstall(action.NewConfiguration())
	install.DryRunStrategy = action.DryRunClient
	install.Replace = true
	install.ReleaseName = ReleaseName(input.ReleaseName)
	install.Namespace = input.Namespace
	rendered, err := install.RunWithContext(ctx, chrt, values)
	if err != nil {
		return nil, fmt.Errorf("helm render failed: %w", err)
	}
	rel, ok := rendered.(*release.Release)
	if !ok {
		return nil, fmt.Errorf("helm render failed: unexpected release type %T", rendered)
	}
	// Like `helm template`, hook manifests are rendered with the chart.
	manifests := []string{rel.Manifest}
	for _, hook := range rel.Hooks {
		manifests = append(manifests, hook.Manifest)
	}
	return yamlrenderer.Decode([]byte(strings.Join(manifests, "\n---\n")))
}

// loadValues merges values files, given relative to the chart directory, in
// order, decrypting SOPS files. Every file must lie inside the workspace.
func loadValues(workspace, dir string, files []string, decrypt func(string, []byte) ([]byte, error)) (map[string]any, error) {
	values := map[string]any{}
	for _, file := range files {
		if filepath.IsAbs(file) {
			return nil, fmt.Errorf("helm values file %s must be relative", file)
		}
		path := filepath.Join(dir, file)
		inside, err := renderer.Within(workspace, path)
		if err != nil {
			return nil, fmt.Errorf("read helm values file %s: %w", file, err)
		}
		if !inside {
			return nil, fmt.Errorf("helm values file %s is outside the workspace", file)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read helm values file %s: %w", file, err)
		}
		if decrypt != nil {
			if raw, err = decrypt(renderer.Relative(workspace, path), raw); err != nil {
				return nil, err
			}
		}
		loaded, err := valuesloader.LoadValues(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("parse helm values file %s: %w", file, err)
		}
		values = valuesloader.MergeMaps(values, loaded)
	}
	return values, nil
}

var _ renderer.Renderer = Renderer{}
