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
