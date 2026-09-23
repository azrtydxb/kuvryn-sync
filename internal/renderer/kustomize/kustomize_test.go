package kustomize

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

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRenderOverlayWithPatch(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, map[string]string{
		"base/kustomization.yaml": "resources:\n- configmap.yaml\n",
		"base/configmap.yaml":     "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: settings\ndata:\n  level: info\n",
		"overlays/prod/kustomization.yaml": `resources:
- ../../base
namePrefix: prod-
patches:
- patch: |-
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: settings
    data:
      level: warn
`,
	})

	objects, err := Renderer{}.Render(context.Background(), renderer.Input{Workspace: workspace, Path: "overlays/prod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0].GetName() != "prod-settings" {
		t.Fatalf("objects = %#v", objects)
	}
	if level := objects[0].Object["data"].(map[string]any)["level"]; level != "warn" {
		t.Fatalf("patched level = %v", level)
	}
}

func TestRenderCannotReadOutsideWorkspace(t *testing.T) {
	outside := t.TempDir()
	write(t, outside, map[string]string{
		"secret/kustomization.yaml": "resources:\n- configmap.yaml\n",
		"secret/configmap.yaml":     "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: stolen\n",
	})
	workspace := t.TempDir()
	rel, err := filepath.Rel(filepath.Join(workspace, "app"), filepath.Join(outside, "secret"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, workspace, map[string]string{"app/kustomization.yaml": "resources:\n- " + rel + "\n"})

	if _, err := (Renderer{}).Render(context.Background(), renderer.Input{Workspace: workspace, Path: "app"}); err == nil {
		t.Fatal("a base outside the workspace was rendered")
	}
}

func TestRenderRejectsSymlinkOutOfWorkspace(t *testing.T) {
	outside := t.TempDir()
	write(t, outside, map[string]string{"token": "secret"})
	workspace := t.TempDir()
	write(t, workspace, map[string]string{"app/kustomization.yaml": "configMapGenerator:\n- name: leak\n  files:\n  - token\n"})
	if err := os.Symlink(filepath.Join(outside, "token"), filepath.Join(workspace, "app", "token")); err != nil {
		t.Fatal(err)
	}

	_, err := Renderer{}.Render(context.Background(), renderer.Input{Workspace: workspace, Path: "app"})
	if err == nil || !strings.Contains(err.Error(), "outside the workspace") {
		t.Fatalf("err = %v", err)
	}
}

func TestRenderRejectsTraversalPath(t *testing.T) {
	if _, err := (Renderer{}).Render(context.Background(), renderer.Input{Workspace: t.TempDir(), Path: "../outside"}); err == nil {
		t.Fatal("traversal path accepted")
	}
}

func TestRenderDecryptsSOPSBeforeTransforms(t *testing.T) {
	key := decrypttest.Identity(t)
	workspace := t.TempDir()
	write(t, workspace, map[string]string{
		"app/kustomization.yaml": "namePrefix: prod-\nresources:\n- secret.yaml\n",
		"app/secret.yaml":        string(decrypttest.Encrypt(t, key.Recipient().String(), "apiVersion: v1\nkind: Secret\nmetadata:\n  name: db\nstringData:\n  password: hunter2\n")),
	})
	decryptor, err := decrypt.New(key.String())
	if err != nil {
		t.Fatal(err)
	}
	objects, err := Renderer{}.Render(context.Background(), renderer.Input{Workspace: workspace, Path: "app", Decrypt: decryptor.File})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0].GetName() != "prod-db" {
		t.Fatalf("objects = %#v", objects)
	}
	if objects[0].Object["stringData"].(map[string]any)["password"] != "hunter2" || objects[0].Object["sops"] != nil {
		t.Fatalf("secret not decrypted: %#v", objects[0].Object)
	}
}
