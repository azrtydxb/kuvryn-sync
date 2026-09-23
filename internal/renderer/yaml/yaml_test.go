package yaml

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/solder/internal/decrypt"
	"github.com/azrtydxb/solder/internal/decrypt/decrypttest"
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

func TestRendererNamesFilesByTheirRepositoryPath(t *testing.T) {
	key := decrypttest.Identity(t)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "app"), 0o700); err != nil {
		t.Fatal(err)
	}
	encrypted := decrypttest.Encrypt(t, key.Recipient().String(), "apiVersion: v1\nkind: Secret\nmetadata:\n  name: db\nstringData:\n  password: hunter2\n")
	if err := os.WriteFile(filepath.Join(workspace, "app", "secret.yaml"), encrypted, 0o600); err != nil {
		t.Fatal(err)
	}
	var none *decrypt.Decryptor
	_, err := (Renderer{}).Render(context.Background(), renderer.Input{Workspace: workspace, Path: "app", Decrypt: none.File})
	if err == nil || !strings.HasPrefix(err.Error(), "app/secret.yaml is SOPS-encrypted") || strings.Contains(err.Error(), workspace) {
		t.Fatalf("err = %v", err)
	}
}

func TestRendererRejectsPathTraversal(t *testing.T) {
	_, err := (Renderer{}).Render(context.Background(), renderer.Input{Workspace: t.TempDir(), Path: "../outside"})
	if err == nil {
		t.Fatal("expected path traversal error")
	}
}
