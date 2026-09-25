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
	"os"
	"path/filepath"
	"testing"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// TestConfigFileCannotLinkOutOfTheRepository proves repository content cannot
// make the controller create Applications from a file elsewhere on its own
// filesystem. The config path was checked lexically, but os.ReadFile followed a
// symbolic link at the file or at any directory above it. The workspace is
// built on disk because the Git cache already refuses such links. Proved by:
// removing the resolved-path check reads both linked configs below as
// Applications.
func TestConfigFileCannotLinkOutOfTheRepository(t *testing.T) {
	outside := t.TempDir()
	foreign := "kind: Application\nmetadata:\n  name: planted\nspec:\n  source:\n    path: x\n"
	if err := os.WriteFile(filepath.Join(outside, ".ksync.yaml"), []byte(foreign), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, ".ksync.yaml"), filepath.Join(workspace, ".ksync.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "conf")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "apps"), 0o700); err != nil {
		t.Fatal(err)
	}
	own := "kind: Application\nmetadata:\n  name: mine\nspec:\n  source:\n    path: apps\n"
	if err := os.WriteFile(filepath.Join(workspace, "apps", ".ksync.yaml"), []byte(own), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := &corev1alpha1.Repository{}
	repo.Name = "r"

	for _, linked := range []string{".ksync.yaml", "conf/.ksync.yaml"} {
		apps, _, err := applicationsFromConfigFile(repo, workspace, linked)
		if err == nil || len(apps) != 0 {
			t.Fatalf("%s links out of the repository and must be refused, got %v %v", linked, apps, err)
		}
	}
	apps, found, err := applicationsFromConfigFile(repo, workspace, "apps/.ksync.yaml")
	if err != nil || !found || len(apps) != 1 || apps[0].Name != "mine" {
		t.Fatalf("a regular config file in the repository must still load: %v %v %v", apps, found, err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "inner"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "apps", ".ksync.yaml"), filepath.Join(workspace, "inner", ".ksync.yaml")); err != nil {
		t.Fatal(err)
	}
	apps, found, err = applicationsFromConfigFile(repo, workspace, "inner/.ksync.yaml")
	if err != nil || !found || len(apps) != 1 || apps[0].Name != "mine" {
		t.Fatalf("a link that stays inside the repository must still load: %v %v %v", apps, found, err)
	}
	apps, found, err = applicationsFromConfigFile(repo, workspace, "missing/.ksync.yaml")
	if err != nil || found || len(apps) != 0 {
		t.Fatalf("a missing config file must mean no config: %v %v %v", apps, found, err)
	}
}
