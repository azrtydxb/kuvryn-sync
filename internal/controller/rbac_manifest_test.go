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

package controller

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	sigsyaml "sigs.k8s.io/yaml"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/brand"
	"github.com/azrtydxb/kuvryn-sync/internal/renderer"
	helmrenderer "github.com/azrtydxb/kuvryn-sync/internal/renderer/helm"
)

// managerRules returns the rules of the manager ClusterRole in a manifest as
// sorted "group/resource:verb" grants.
func managerRules(t *testing.T, path, nameSuffix string) []string {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	decoder := utilyaml.NewYAMLOrJSONDecoder(file, 4096)
	for {
		role := rbacv1.ClusterRole{}
		if err := decoder.Decode(&role); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if role.Kind != "ClusterRole" || !strings.HasSuffix(role.Name, nameSuffix) {
			continue
		}
		grants := []string{}
		for _, rule := range role.Rules {
			for _, group := range rule.APIGroups {
				for _, resource := range rule.Resources {
					for _, verb := range rule.Verbs {
						grants = append(grants, group+"/"+resource+":"+verb)
					}
				}
			}
		}
		sort.Strings(grants)
		return grants
	}
	t.Fatalf("no ClusterRole ending in %q in %s", nameSuffix, path)
	return nil
}

func TestControllerRoleOnlyWritesKuvrynSyncObjects(t *testing.T) {
	for _, grant := range managerRules(t, "config/rbac/role.yaml", "manager-role") {
		verb := grant[strings.LastIndex(grant, ":")+1:]
		switch verb {
		case "get", "list", "watch", "impersonate":
			continue
		}
		if strings.HasPrefix(grant, "sync.kuvryn.io/") || strings.HasPrefix(grant, "/events:") {
			continue
		}
		t.Errorf("controller role grants %s; managed resources must be changed as the Application's service account", grant)
	}
}

func TestHelmChartRoleMatchesGeneratedRole(t *testing.T) {
	generated := strings.Join(managerRules(t, "config/rbac/role.yaml", "manager-role"), "\n")
	chart := strings.Join(managerRules(t, "charts/kuvryn-sync/templates/rbac.yaml", "-manager"), "\n")
	if generated != chart {
		t.Fatalf("charts/kuvryn-sync/templates/rbac.yaml drifted from config/rbac/role.yaml\ngenerated:\n%s\nchart:\n%s", generated, chart)
	}
}

// Two chart releases in one namespace must never route to each other's
// manager, so every Service selects the release, and the pods carry it.
func TestHelmChartServicesSelectTheirOwnRelease(t *testing.T) {
	const instance = "app.kubernetes.io/instance: {{ .Release.Name }}"
	templates, err := filepath.Glob(filepath.Join("..", "..", "charts", "kuvryn-sync", "templates", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	services := 0
	for _, path := range templates {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, document := range strings.Split(string(contents), "\n---") {
			switch {
			case strings.Contains(document, "\nkind: Service\n"):
				services++
				selector := document[strings.Index(document, "\n  selector:"):]
				if end := strings.Index(selector, "\n  ports:"); end >= 0 {
					selector = selector[:end]
				}
				if !strings.Contains(selector, instance) {
					t.Errorf("Service in %s does not select %q", filepath.Base(path), instance)
				}
			case strings.Contains(document, "\nkind: Deployment\n") && !strings.Contains(document, "        "+instance):
				t.Errorf("Deployment in %s does not label its pods with %q", filepath.Base(path), instance)
			}
		}
	}
	if services == 0 {
		t.Fatal("found no Services in the chart templates")
	}
}

// A Go bump must reach every place that picks a toolchain, or contributors
// and image builds run an older Go than go.mod requires.
func TestGoVersionMatchesBuildImages(t *testing.T) {
	read := func(path string) string {
		contents, err := os.ReadFile(filepath.Join("..", "..", path))
		if err != nil {
			t.Fatal(err)
		}
		return string(contents)
	}
	module := ""
	for line := range strings.SplitSeq(read("go.mod"), "\n") {
		if version, ok := strings.CutPrefix(line, "go "); ok {
			parts := strings.SplitN(version, ".", 3)
			module = parts[0] + "." + parts[1]
		}
	}
	if module == "" {
		t.Fatal("go.mod has no go directive")
	}
	for _, path := range []string{"Dockerfile", ".devcontainer/devcontainer.json"} {
		if !strings.Contains(read(path), "golang:"+module) {
			t.Errorf("%s does not use golang:%s, the Go version go.mod requires", path, module)
		}
	}
}

// The CRD must accept what rendering accepts: an empty releaseName means the
// default, and the pattern must agree with Helm's rule.
func TestHelmReleaseNameSchemaMatchesRendering(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "config", "crd", "bases", "sync.kuvryn.io_applications.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	crd := map[string]any{}
	if err := sigsyaml.Unmarshal(contents, &crd); err != nil {
		t.Fatal(err)
	}
	var find func(node any) string
	find = func(node any) string {
		switch value := node.(type) {
		case map[string]any:
			if props, ok := value["properties"].(map[string]any); ok {
				if release, ok := props["releaseName"].(map[string]any); ok {
					if pattern, ok := release["pattern"].(string); ok {
						return pattern
					}
				}
			}
			for _, child := range value {
				if found := find(child); found != "" {
					return found
				}
			}
		case []any:
			for _, child := range value {
				if found := find(child); found != "" {
					return found
				}
			}
		}
		return ""
	}
	pattern := find(crd)
	if pattern == "" {
		t.Fatal("no releaseName pattern in the Application CRD")
	}
	schema := regexp.MustCompile(pattern)
	for name, valid := range map[string]bool{"": true, "payments": true, "api.v2": true, "Payments": false, "-bad": false, "a_b": false} {
		if got := schema.MatchString(name); got != valid {
			t.Errorf("CRD pattern on %q = %v, want %v", name, got, valid)
		}
		app := &corev1alpha1.Application{Spec: corev1alpha1.ApplicationSpec{Source: corev1alpha1.ApplicationSource{
			Render: corev1alpha1.RenderSpec{Type: corev1alpha1.RenderTypeHelm, Helm: &corev1alpha1.HelmRenderSpec{ReleaseName: name}},
		}}}
		if got := validateHelmReleaseName(app) == nil; got != valid {
			t.Errorf("rendering accepts %q = %v, want %v", name, got, valid)
		}
	}
}

// TestHelmChartUsesKuvrynSyncNames renders the chart like `helm template`
// (in process, so no helm binary is needed) and catches a chart that still
// installs the old image or names anything after the old product.
func TestHelmChartUsesKuvrynSyncNames(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	objects, err := helmrenderer.Renderer{}.Render(context.Background(), renderer.Input{
		Workspace: root, Path: filepath.Join("charts", "kuvryn-sync"), ReleaseName: "kuvryn-sync", Namespace: "kuvryn-sync-system",
	})
	if err != nil {
		t.Fatalf("helm template: %v", err)
	}
	chartFile, err := os.ReadFile(filepath.Join(root, "charts", "kuvryn-sync", "Chart.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var chart struct {
		AppVersion string `json:"appVersion"`
	}
	if err := sigsyaml.Unmarshal(chartFile, &chart); err != nil {
		t.Fatal(err)
	}
	wantImage := "ghcr.io/azrtydxb/kuvryn-sync:v" + chart.AppVersion
	var rendered strings.Builder
	images := 0
	for _, obj := range objects {
		out, err := sigsyaml.Marshal(obj.Object)
		if err != nil {
			t.Fatal(err)
		}
		rendered.Write(out)
		if obj.GetKind() != "Deployment" {
			continue
		}
		for _, image := range containerImages(obj.Object) {
			images++
			if image != wantImage {
				t.Errorf("the Deployment runs %s, want %s", image, wantImage)
			}
		}
	}
	if images == 0 {
		t.Error("the chart renders no Deployment container")
	}
	if strings.Contains(strings.ToLower(rendered.String()), brand.OldName) {
		t.Errorf("the rendered chart still says %s:\n%s", brand.OldName, rendered.String())
	}
}

// containerImages returns the image of every container in a workload's pod
// template.
func containerImages(obj map[string]any) []string {
	spec, _ := obj["spec"].(map[string]any)
	template, _ := spec["template"].(map[string]any)
	podSpec, _ := template["spec"].(map[string]any)
	containers, _ := podSpec["containers"].([]any)
	var images []string
	for _, c := range containers {
		if m, ok := c.(map[string]any); ok {
			image, _ := m["image"].(string)
			images = append(images, image)
		}
	}
	return images
}
