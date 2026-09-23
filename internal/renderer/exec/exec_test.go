package exec

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/azrtydxb/solder/internal/renderer"
)

func TestKustomizeRendererUsesBuildOutput(t *testing.T) {
	binary := fakeBinary(t, `#!/bin/sh
if [ "$1" != "build" ]; then exit 2; fi
cat <<'YAML'
apiVersion: v1
kind: ConfigMap
metadata:
  name: from-kustomize
YAML
`)
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "app"), 0o700); err != nil {
		t.Fatal(err)
	}
	objects, err := (KustomizeRenderer{Binary: binary}).Render(context.Background(), renderer.Input{Workspace: workspace, Path: "app"})
	if err != nil {
		t.Fatalf("render kustomize: %v", err)
	}
	if len(objects) != 1 || objects[0].GetName() != "from-kustomize" {
		t.Fatalf("unexpected objects: %#v", objects)
	}
}

func TestHelmRendererUsesTemplateOutput(t *testing.T) {
	binary := fakeBinary(t, `#!/bin/sh
if [ "$1" != "template" ]; then exit 2; fi
cat <<'YAML'
apiVersion: v1
kind: Service
metadata:
  name: from-helm
YAML
`)
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "chart"), 0o700); err != nil {
		t.Fatal(err)
	}
	objects, err := (HelmRenderer{Binary: binary}).Render(context.Background(), renderer.Input{Workspace: workspace, Path: "chart", ReleaseName: "payments"})
	if err != nil {
		t.Fatalf("render helm: %v", err)
	}
	if len(objects) != 1 || objects[0].GetName() != "from-helm" {
		t.Fatalf("unexpected objects: %#v", objects)
	}
}

func TestRenderPathRejectsTraversal(t *testing.T) {
	workspace := t.TempDir()
	if _, err := renderPath(renderer.Input{Workspace: workspace, Path: "../outside"}); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
}

func fakeBinary(t *testing.T, content string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fake binary is Unix-only")
	}
	path := filepath.Join(t.TempDir(), "fake")
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestHelmValuesFilesStayInsideTheWorkspace proves a values file cannot reach
// outside the checkout. Only absolute paths were refused, so a relative path
// with ".." walked out, and a values file that is itself a symbolic link
// pointed anywhere; either handed Helm a file from the controller's
// filesystem, such as its service account token. proved by: joining the
// values file without within() lets both escapes reach the fake helm below.
func TestHelmValuesFilesStayInsideTheWorkspace(t *testing.T) {
	binary := fakeBinary(t, `#!/bin/sh
cat <<'YAML'
apiVersion: v1
kind: Service
metadata:
  name: from-helm
YAML
`)
	outside := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(outside, []byte("secret: stolen\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	chart := filepath.Join(workspace, "chart")
	if err := os.MkdirAll(filepath.Join(chart, "values"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chart, "values", "prod.yaml"), []byte("replicas: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(chart, "values", "linked.yaml")); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(chart, outside)
	if err != nil {
		t.Fatal(err)
	}
	render := func(values string) error {
		_, err := (HelmRenderer{Binary: binary}).Render(context.Background(), renderer.Input{
			Workspace: workspace, Path: "chart", ValuesFiles: []string{values},
		})
		return err
	}
	if err := render("values/prod.yaml"); err != nil {
		t.Fatalf("a values file inside the chart must still render: %v", err)
	}
	for name, values := range map[string]string{"dot-dot": rel, "symlink": "values/linked.yaml"} {
		if err := render(values); err == nil {
			t.Fatalf("%s: a values file resolving outside the workspace must be refused", name)
		}
	}
}

// TestHelmReleaseNameCannotBecomeAFlag proves the release name from the
// Application spec reaches Helm only as a release name. It was unvalidated and
// placed before any "--", so "--post-renderer=<path>" was read as a flag, and
// --post-renderer runs a program. proved by: dropping the name check or the
// "--" lets the injected name through to the fake helm, which records it.
func TestHelmReleaseNameCannotBecomeAFlag(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	binary := fakeBinary(t, `#!/bin/sh
printf '%s\n' "$@" > `+argsFile+`
cat <<'YAML'
apiVersion: v1
kind: Service
metadata:
  name: from-helm
YAML
`)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "chart"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"--post-renderer=/tmp/evil", "-f", "UPPER", strings.Repeat("a", 54)} {
		if _, err := (HelmRenderer{Binary: binary}).Render(context.Background(), renderer.Input{Workspace: workspace, Path: "chart", ReleaseName: name}); err == nil {
			t.Fatalf("release name %q must be refused", name)
		}
	}
	if _, err := (HelmRenderer{Binary: binary}).Render(context.Background(), renderer.Input{Workspace: workspace, Path: "chart", ReleaseName: "payments.v2"}); err != nil {
		t.Fatalf("a valid release name must render: %v", err)
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(got)), "\n")
	if len(args) < 4 || args[0] != "template" || args[len(args)-3] != "--" || args[len(args)-2] != "payments.v2" {
		t.Fatalf("positional arguments must follow --: %q", args)
	}
}
