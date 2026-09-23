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
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

// managerRules returns the rules of the manager ClusterRole in a manifest as
// sorted "group/resource:verb" grants.
func managerRules(t *testing.T, path, nameSuffix string) []string {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	decoder := utilyaml.NewYAMLOrJSONDecoder(file, 4096)
	for {
		role := rbacv1.ClusterRole{}
		if err := decoder.Decode(&role); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if role.Kind != "ClusterRole" || !strings.HasSuffix(role.Name, nameSuffix) {
			continue
		}
		grants := []string{}
		for _, rule := range role.Rules {
			for _, group := range rule.APIGroups {
				for _, resource := range rule.Resources {
					for _, verb := range rule.Verbs {
						grants = append(grants, group+"/"+resource+":"+verb)
					}
				}
			}
		}
		sort.Strings(grants)
		return grants
	}
	t.Fatalf("no ClusterRole ending in %q in %s", nameSuffix, path)
	return nil
}

func TestControllerRoleOnlyWritesSolderObjects(t *testing.T) {
	for _, grant := range managerRules(t, "config/rbac/role.yaml", "manager-role") {
		verb := grant[strings.LastIndex(grant, ":")+1:]
		switch verb {
		case "get", "list", "watch", "impersonate":
			continue
		}
		if strings.HasPrefix(grant, "solder.io/") || strings.HasPrefix(grant, "/events:") {
			continue
		}
		t.Errorf("controller role grants %s; managed resources must be changed as the Application's service account", grant)
	}
}

func TestHelmChartRoleMatchesGeneratedRole(t *testing.T) {
	generated := strings.Join(managerRules(t, "config/rbac/role.yaml", "manager-role"), "\n")
	chart := strings.Join(managerRules(t, "charts/solder/templates/rbac.yaml", "-manager"), "\n")
	if generated != chart {
		t.Fatalf("charts/solder/templates/rbac.yaml drifted from config/rbac/role.yaml\ngenerated:\n%s\nchart:\n%s", generated, chart)
	}
}

// Two chart releases in one namespace must never route to each other's
// manager, so every Service selects the release, and the pods carry it.
func TestHelmChartServicesSelectTheirOwnRelease(t *testing.T) {
	const instance = "app.kubernetes.io/instance: {{ .Release.Name }}"
	templates, err := filepath.Glob(filepath.Join("..", "..", "charts", "solder", "templates", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	services := 0
	for _, path := range templates {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, document := range strings.Split(string(contents), "\n---") {
			switch {
			case strings.Contains(document, "\nkind: Service\n"):
				services++
				selector := document[strings.Index(document, "\n  selector:"):]
				if end := strings.Index(selector, "\n  ports:"); end >= 0 {
					selector = selector[:end]
				}
				if !strings.Contains(selector, instance) {
					t.Errorf("Service in %s does not select %q", filepath.Base(path), instance)
				}
			case strings.Contains(document, "\nkind: Deployment\n") && !strings.Contains(document, "        "+instance):
				t.Errorf("Deployment in %s does not label its pods with %q", filepath.Base(path), instance)
			}
		}
	}
	if services == 0 {
		t.Fatal("found no Services in the chart templates")
	}
}
