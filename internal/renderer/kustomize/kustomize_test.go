package kustomize

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func TestRenderSkipsLinksItDoesNotRead(t *testing.T) {
	outside := t.TempDir()
	write(t, outside, map[string]string{"token": "secret"})
	workspace := t.TempDir()
	write(t, workspace, map[string]string{
		"app/kustomization.yaml": "resources:\n- configmap.yaml\n",
		"app/configmap.yaml":     "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: settings\n",
	})
	for name, target := range map[string]string{"dangling": "missing/file", "loop": "loop", "elsewhere": filepath.Join(outside, "token")} {
		if err := os.Symlink(target, filepath.Join(workspace, name)); err != nil {
			t.Fatal(err)
		}
	}
	objects, err := Renderer{}.Render(context.Background(), renderer.Input{Workspace: workspace, Path: "app"})
	if err != nil || len(objects) != 1 {
		t.Fatalf("objects = %d, err = %v", len(objects), err)
	}
}

func TestRenderRefusesRemoteReferences(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: fetched\n"))
	}))
	defer server.Close()
	for name, kustomization := range map[string]string{
		"http resource":      "resources:\n- " + server.URL + "/configmap.yaml\n",
		"github base":        "resources:\n- github.com/kubernetes-sigs/kustomize//examples/helloWorld?ref=v3.3.1\n",
		"scp-style base":     "bases:\n- git@github.com:acme/platform.git//base\n",
		"generator file url": "configMapGenerator:\n- name: fetched\n  files:\n  - config=" + server.URL + "/config\n",
		"patch path url":     "resources:\n- configmap.yaml\npatches:\n- path: " + server.URL + "/patch.yaml\n",
	} {
		t.Run(name, func(t *testing.T) {
			workspace := t.TempDir()
			write(t, workspace, map[string]string{
				"app/kustomization.yaml": kustomization,
				"app/configmap.yaml":     "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: settings\n",
			})
			_, err := Renderer{}.Render(context.Background(), renderer.Input{Workspace: workspace, Path: "app"})
			if err == nil || !strings.Contains(err.Error(), "refers to remote") {
				t.Fatalf("err = %v", err)
			}
			if requests.Load() != 0 {
				t.Fatalf("kustomize fetched %d remote files", requests.Load())
			}
		})
	}
}

func TestRenderIgnoresUnrelatedSOPSFiles(t *testing.T) {
	key := decrypttest.Identity(t)
	workspace := t.TempDir()
	write(t, workspace, map[string]string{
		"app/kustomization.yaml":        "resources:\n- configmap.yaml\n",
		"app/configmap.yaml":            "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: settings\n",
		"other-team/secret.yaml":        string(decrypttest.Encrypt(t, key.Recipient().String(), "apiVersion: v1\nkind: Secret\nmetadata:\n  name: db\nstringData:\n  password: hunter2\n")),
		"other-team/broken.yaml":        "{not: yaml",
		"other-team/kustomization.yaml": "resources:\n- https://example.com/remote.yaml\n",
	})
	var none *decrypt.Decryptor
	objects, err := Renderer{}.Render(context.Background(), renderer.Input{Workspace: workspace, Path: "app", Decrypt: none.File})
	if err != nil || len(objects) != 1 {
		t.Fatalf("objects = %d, err = %v", len(objects), err)
	}
}

func TestRenderStopsWhenContextIsDone(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, map[string]string{"app/kustomization.yaml": "resources: []\n"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Renderer{}).Render(ctx, renderer.Input{Workspace: workspace, Path: "app"}); !errors.Is(err, context.Canceled) {
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
