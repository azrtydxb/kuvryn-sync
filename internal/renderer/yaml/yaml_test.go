package yaml

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/azrtydxb/solder/internal/renderer"
)

func TestRendererDecodesMultiDocumentYAML(t *testing.T) {
	workspace := t.TempDir()
	manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
---
apiVersion: v1
kind: Secret
metadata:
  name: app-secret
`
	if err := os.WriteFile(filepath.Join(workspace, "objects.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	objects, err := (Renderer{}).Render(context.Background(), renderer.Input{Workspace: workspace, Path: "."})
	if err != nil {
		t.Fatalf("render YAML: %v", err)
	}
	if len(objects) != 2 {
		t.Fatalf("objects = %d, want 2", len(objects))
	}
	if objects[0].GetName() != "app-config" || objects[1].GetName() != "app-secret" {
		t.Fatalf("unexpected object order: %s, %s", objects[0].GetName(), objects[1].GetName())
	}
}

func TestRendererRejectsPathTraversal(t *testing.T) {
	_, err := (Renderer{}).Render(context.Background(), renderer.Input{Workspace: t.TempDir(), Path: "../outside"})
	if err == nil {
		t.Fatal("expected path traversal error")
	}
}

// TestRendererIgnoresLinksOutOfTheWorkspace proves repository content cannot
// make the controller read, and then apply, YAML from elsewhere on its own
// filesystem. WalkDir listed a linked manifest and os.ReadFile followed it,
// and a render path that was itself a link passed the lexical check. proved
// by: dropping the symlink skip renders "stolen"; dropping resolvedInside
// renders the linked directory.
func TestRendererIgnoresLinksOutOfTheWorkspace(t *testing.T) {
	outside := t.TempDir()
	stolen := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: stolen\n"
	if err := os.WriteFile(filepath.Join(outside, "cm.yaml"), []byte(stolen), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	app := filepath.Join(workspace, "app")
	if err := os.MkdirAll(app, 0o700); err != nil {
		t.Fatal(err)
	}
	mine := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: mine\n"
	if err := os.WriteFile(filepath.Join(app, "cm.yaml"), []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "cm.yaml"), filepath.Join(app, "leak.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "linkdir")); err != nil {
		t.Fatal(err)
	}

	objects, err := (Renderer{}).Render(context.Background(), renderer.Input{Workspace: workspace, Path: "app"})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range objects {
		if o.GetName() == "stolen" {
			t.Fatal("a manifest linked from outside the workspace must not be rendered")
		}
	}
	if len(objects) != 1 || objects[0].GetName() != "mine" {
		t.Fatalf("the workspace's own manifest must still render: %v", objects)
	}
	if _, err := (Renderer{}).Render(context.Background(), renderer.Input{Workspace: workspace, Path: "linkdir"}); err == nil {
		t.Fatal("a render path that links out of the workspace must be refused")
	}
}
