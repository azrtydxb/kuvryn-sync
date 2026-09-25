package brand

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCRDsUseTheKuvrynSyncGroup(t *testing.T) {
	want := []string{"applications", "repositories", "revisions", "healthchecks", "notificationsinks", "imagepolicies"}
	for _, plural := range want {
		path := filepath.Join("..", "..", "config", "crd", "bases", "sync.kuvryn.io_"+plural+".yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("missing CRD %s: %v", path, err)
		}
		if !strings.Contains(string(data), "group: sync.kuvryn.io") {
			t.Errorf("%s is not in group sync.kuvryn.io", path)
		}
	}
	matches, _ := filepath.Glob(filepath.Join("..", "..", "config", "crd", "bases", "solder.io_*.yaml"))
	if len(matches) > 0 {
		t.Errorf("old CRDs remain: %v", matches)
	}
}
