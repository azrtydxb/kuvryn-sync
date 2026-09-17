package exec

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
