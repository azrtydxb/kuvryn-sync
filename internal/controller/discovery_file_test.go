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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// TestDiscoveryReadsKsyncYaml catches a discovery that still reads the old
// .solder.yaml name, or never learned the new .ksync.yaml one.
func TestDiscoveryReadsKsyncYaml(t *testing.T) {
	dir := t.TempDir()
	app := "applications:\n  - name: web\n    source:\n      path: web\n"
	if err := os.WriteFile(filepath.Join(dir, ".ksync.yaml"), []byte(app), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"}}
	apps, found, err := applicationsFromConfigFile(repo, dir, ".ksync.yaml")
	if err != nil || !found || len(apps) != 1 {
		t.Fatalf("apps=%v found=%v err=%v, want one Application from .ksync.yaml", apps, found, err)
	}
	old := t.TempDir()
	if err := os.WriteFile(filepath.Join(old, ".solder.yaml"), []byte(app), 0o600); err != nil {
		t.Fatal(err)
	}
	paths, _ := configPaths(repo)
	for _, p := range paths {
		if _, found, _ := applicationsFromConfigFile(repo, old, p); found {
			t.Fatalf("a repository with only .solder.yaml discovered %s", p)
		}
	}
}
