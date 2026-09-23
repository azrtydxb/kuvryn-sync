package helm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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

const configMapTemplate = `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}-{{ .Chart.Name }}
  namespace: {{ .Release.Namespace }}
data:
  level: {{ .Values.level | quote }}
`

func chartWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	write(t, workspace, map[string]string{
		"chart/Chart.yaml":                  "apiVersion: v2\nname: app\nversion: 0.1.0\ndependencies:\n- name: sub\n  version: 0.1.0\n",
		"chart/values.yaml":                 "level: info\n",
		"chart/templates/configmap.yaml":    configMapTemplate,
		"chart/templates/hook.yaml":         "apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: migrate\n  annotations:\n    helm.sh/hook: pre-install\nspec:\n  template:\n    spec:\n      restartPolicy: Never\n      containers: [{name: m, image: busybox}]\n",
		"chart/charts/sub/Chart.yaml":       "apiVersion: v2\nname: sub\nversion: 0.1.0\n",
		"chart/charts/sub/values.yaml":      "level: debug\n",
		"chart/charts/sub/templates/c.yaml": configMapTemplate,
		"envs/prod/values.yaml":             "level: warn\n",
	})
	return workspace
}

func byName(objects []unstructured.Unstructured) map[string]unstructured.Unstructured {
	out := map[string]unstructured.Unstructured{}
	for _, obj := range objects {
		out[obj.GetName()] = obj
	}
	return out
}

func TestRenderChartWithValuesSubchartNamespaceAndHooks(t *testing.T) {
	workspace := chartWorkspace(t)
	objects, err := Renderer{}.Render(context.Background(), renderer.Input{
		Workspace: workspace, Path: "chart", ReleaseName: "payments", Namespace: "payments-prod",
		ValuesFiles: []string{"../envs/prod/values.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := byName(objects)
	app, ok := got["payments-app"]
	if !ok {
		t.Fatalf("objects = %v", got)
	}
	if level := app.Object["data"].(map[string]any)["level"]; level != "warn" {
		t.Fatalf("values override not applied: level = %v", level)
	}
	if app.GetNamespace() != "payments-prod" {
		t.Fatalf("release namespace = %q", app.GetNamespace())
	}
	if sub, ok := got["payments-sub"]; !ok || sub.Object["data"].(map[string]any)["level"] != "debug" {
		t.Fatalf("vendored subchart not rendered with its values: %v", got["payments-sub"])
	}
	if _, ok := got["migrate"]; !ok {
		t.Fatal("hook manifest was not rendered, unlike helm template")
	}
}

func TestRenderRejectsValuesOutsideWorkspace(t *testing.T) {
	outside := t.TempDir()
	write(t, outside, map[string]string{"values.yaml": "level: stolen\n"})
	workspace := chartWorkspace(t)
	rel, err := filepath.Rel(filepath.Join(workspace, "chart"), filepath.Join(outside, "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = Renderer{}.Render(context.Background(), renderer.Input{Workspace: workspace, Path: "chart", ValuesFiles: []string{rel}})
	if err == nil || !strings.Contains(err.Error(), "outside the workspace") {
		t.Fatalf("err = %v", err)
	}
}

func TestRenderRejectsSymlinkOutOfWorkspace(t *testing.T) {
	outside := t.TempDir()
	write(t, outside, map[string]string{"token": "secret"})
	workspace := chartWorkspace(t)
	if err := os.Symlink(filepath.Join(outside, "token"), filepath.Join(workspace, "chart", "token")); err != nil {
		t.Fatal(err)
	}
	_, err := Renderer{}.Render(context.Background(), renderer.Input{Workspace: workspace, Path: "chart"})
	if err == nil || !strings.Contains(err.Error(), "outside the workspace") {
		t.Fatalf("err = %v", err)
	}
}

func TestRenderDecryptsSOPSValuesFiles(t *testing.T) {
	key := decrypttest.Identity(t)
	workspace := t.TempDir()
	write(t, workspace, map[string]string{
		"chart/Chart.yaml":               "apiVersion: v2\nname: app\nversion: 0.1.0\n",
		"chart/values.yaml":              "data:\n  password: none\n",
		"chart/templates/configmap.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: app\ndata:\n  password: {{ .Values.data.password | quote }}\n",
		"envs/prod/secrets.yaml":         string(decrypttest.Encrypt(t, key.Recipient().String(), "data:\n  password: hunter2\n")),
	})
	input := renderer.Input{Workspace: workspace, Path: "chart", ValuesFiles: []string{"../envs/prod/secrets.yaml"}}

	var none *decrypt.Decryptor
	input.Decrypt = none.File
	if _, err := (Renderer{}).Render(context.Background(), input); err == nil || !strings.Contains(err.Error(), "envs/prod/secrets.yaml is SOPS-encrypted") {
		t.Fatalf("encrypted values without decryption: err = %v", err)
	}

	decryptor, err := decrypt.New(key.String())
	if err != nil {
		t.Fatal(err)
	}
	input.Decrypt = decryptor.File
	objects, err := Renderer{}.Render(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0].Object["data"].(map[string]any)["password"] != "hunter2" {
		t.Fatalf("values not decrypted: %#v", objects)
	}
}

func TestRenderReportsMissingDependencies(t *testing.T) {
	workspace := chartWorkspace(t)
	if err := os.RemoveAll(filepath.Join(workspace, "chart", "charts")); err != nil {
		t.Fatal(err)
	}
	_, err := Renderer{}.Render(context.Background(), renderer.Input{Workspace: workspace, Path: "chart"})
	if err == nil || !strings.Contains(err.Error(), "helm dependency build") {
		t.Fatalf("err = %v", err)
	}
}
