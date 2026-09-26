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

package renderer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContainedRefusesARenderPathThroughAnEscapingLink(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(filepath.Join(outside, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "sub", "secret.yaml"), []byte("kind: ConfigMap\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "link")); err != nil {
		t.Fatal(err)
	}

	if err := Contained(workspace, filepath.Join(workspace, "link", "sub")); err == nil {
		t.Fatal("render path through a link out of the workspace was accepted")
	}
}

func TestContainedAcceptsAPathInsideTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "app"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("app", filepath.Join(workspace, "current")); err != nil {
		t.Fatal(err)
	}
	if err := Contained(workspace, filepath.Join(workspace, "current")); err != nil {
		t.Fatalf("path inside the workspace was refused: %v", err)
	}
}

// Catches the yaml and helm renderers failing on a broken link the Git cache
// kept because it points inside the repository, which kustomize skips.
func TestContainedSkipsABrokenLinkInsideTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing.md", filepath.Join(workspace, "app", "README.md")); err != nil {
		t.Fatal(err)
	}
	if err := Contained(workspace, filepath.Join(workspace, "app")); err != nil {
		t.Fatalf("refused a broken link inside the workspace: %v", err)
	}
}

func TestContainedRefusesABrokenLinkOutOfTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"../../outside.yaml", "/nonexistent/secret.yaml"} {
		link := filepath.Join(workspace, "app", "values.yaml")
		_ = os.Remove(link)
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if err := Contained(workspace, filepath.Join(workspace, "app")); err == nil {
			t.Fatalf("accepted a broken link to %s", target)
		}
	}
}
