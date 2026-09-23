package helm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	chartv2loader "helm.sh/helm/v4/pkg/chart/v2/loader"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	repo "helm.sh/helm/v4/pkg/repo/v1"

	"github.com/azrtydxb/solder/internal/imagepolicy/registrytest"
	"github.com/azrtydxb/solder/internal/renderer"
)

// packagedChart builds the test chart as a .tgz and returns its bytes and metadata.
func packagedChart(t *testing.T, dir string) ([]byte, *chartv2.Chart) {
	t.Helper()
	src := t.TempDir()
	write(t, src, map[string]string{
		"app/Chart.yaml":               "apiVersion: v2\nname: app\nversion: 0.1.0\n",
		"app/values.yaml":              "level: info\n",
		"app/templates/configmap.yaml": configMapTemplate,
	})
	loaded, err := chartv2loader.LoadDir(filepath.Join(src, "app"))
	if err != nil {
		t.Fatal(err)
	}
	archive, err := chartutil.Save(loaded, dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	return data, loaded
}

func sha(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestPullFromHTTPRepositoryCachesAndRenders(t *testing.T) {
	repoDir := t.TempDir()
	archive, _ := packagedChart(t, repoDir)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if user, pass, ok := r.BasicAuth(); !ok || user != "robot" || pass != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.FileServer(http.Dir(repoDir)).ServeHTTP(w, r)
	}))
	defer server.Close()
	index, err := repo.IndexDirectory(repoDir, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.WriteFile(filepath.Join(repoDir, "index.yaml"), 0o600); err != nil {
		t.Fatal(err)
	}

	cache := t.TempDir()
	src := ChartSource{Repository: server.URL, Name: "app", Version: "0.1.0", Username: "robot", Password: "pw", PlainHTTP: true}
	path, digest, err := Pull(cache, "payments", src)
	if err != nil {
		t.Fatal(err)
	}
	if digest != sha(archive) {
		t.Fatalf("digest = %s, want %s", digest, sha(archive))
	}
	served := requests.Load()
	if _, again, err := Pull(cache, "payments", src); err != nil || again != digest || requests.Load() != served {
		t.Fatalf("second pull was not served from cache: %v, %d requests", err, requests.Load()-served)
	}

	// The cached archive belongs to these credentials in this namespace.
	anonymous := src
	anonymous.Username, anonymous.Password = "", ""
	if _, _, err := Pull(cache, "payments", anonymous); err == nil {
		t.Fatal("a pull without credentials was served the private chart from cache")
	}
	if _, _, err := Pull(cache, "other", src); err != nil || requests.Load() == served {
		t.Fatalf("another namespace was served from cache: %v", err)
	}

	objects, err := Renderer{}.Render(context.Background(), renderer.Input{
		Workspace: t.TempDir(), ChartPath: path, ReleaseName: "payments", Namespace: "payments",
		Values: map[string]any{"level": "warn"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0].Object["data"].(map[string]any)["level"] != "warn" {
		t.Fatalf("objects = %#v", objects)
	}

	if _, _, err := Pull(t.TempDir(), "payments", ChartSource{Repository: server.URL, Name: "app", Version: "0.1.0", Username: "robot", Password: "wrong", PlainHTTP: true}); err == nil {
		t.Fatal("pulled with wrong credentials")
	}
	if _, _, err := Pull(t.TempDir(), "payments", ChartSource{Repository: "http://charts.example.com", Name: "app", Version: "0.1.0"}); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("plain http repository: %v", err)
	}
}

func TestPullRefusesVersionRanges(t *testing.T) {
	for _, version := range []string{">=1.0.0", "^1.2.0", "~1.2", "1.x", "1.0", "1.0.0 || 2.0.0"} {
		_, _, err := Pull(t.TempDir(), "payments", ChartSource{Repository: "https://charts.example.com", Name: "app", Version: version})
		if err == nil || !strings.Contains(err.Error(), "exact semantic version") {
			t.Fatalf("version %q: err = %v", version, err)
		}
	}
}

func TestPullFromOCIRegistry(t *testing.T) {
	registry := registrytest.New("robot", "pw")
	defer registry.Close()
	archive, loaded := packagedChart(t, t.TempDir())
	config, err := json.Marshal(loaded.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	registry.PushArtifact("charts/app", "0.1.0", "application/vnd.cncf.helm.config.v1+json", config, "application/vnd.cncf.helm.chart.content.v1.tar+gzip", archive)

	_, digest, err := Pull(t.TempDir(), "payments", ChartSource{Repository: "oci://" + registry.Host() + "/charts", Name: "app", Version: "0.1.0", Username: "robot", Password: "pw", PlainHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	if digest != sha(archive) {
		t.Fatalf("digest = %s, want %s", digest, sha(archive))
	}
}
