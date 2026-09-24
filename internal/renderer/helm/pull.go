package helm

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/registry"
)

// ChartSource is a chart in an https Helm repository or an oci:// registry.
type ChartSource struct {
	Repository string
	Name       string
	Version    string
	Username   string
	Password   string
	// PlainHTTP talks to the repository over HTTP; only tests use it.
	PlainHTTP bool
	// CAFile trusts an additional certificate authority for the repository.
	CAFile string
}

var pullLocks sync.Map

// Pull downloads a pinned chart archive into cacheDir for an Application in
// namespace, reusing an earlier download of the same repository, name, and
// version only for the same namespace and credentials, and returns the
// archive path and its sha256 digest. Helm's user configuration, cached
// credentials, and plugins are never used.
func Pull(cacheDir, namespace string, src ChartSource) (string, string, error) {
	if !src.PlainHTTP && !strings.HasPrefix(src.Repository, "https://") && !strings.HasPrefix(src.Repository, "oci://") {
		return "", "", fmt.Errorf("chart repository must use https:// or oci://")
	}
	// Helm reads the version as a constraint, and a cached range would never
	// pick up a newer match. Charts such as cert-manager's use a v prefix.
	if _, err := semver.StrictNewVersion(strings.TrimPrefix(src.Version, "v")); err != nil {
		return "", "", fmt.Errorf("chart version %q must be an exact semantic version, not a range: %w", src.Version, err)
	}
	// A private chart pulled with one namespace's credentials must never be
	// served to another namespace, or to a pull without those credentials.
	credentials := sha256.Sum256([]byte(src.Username + "\x00" + src.Password))
	sum := sha256.Sum256([]byte(src.Repository + "\x00" + src.Name + "\x00" + src.Version + "\x00" + namespace + "\x00" + hex.EncodeToString(credentials[:])))
	dest := filepath.Join(cacheDir, hex.EncodeToString(sum[:]))
	lock, _ := pullLocks.LoadOrStore(dest, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	if archive, err := findArchive(dest); err == nil {
		// Mark the cached chart as used, so PruneCache keeps it.
		now := time.Now()
		if err := os.Chtimes(dest, now, now); err != nil {
			return "", "", err
		}
		return digestFile(archive)
	}
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return "", "", err
	}
	settings := cli.New()
	settings.RepositoryConfig = filepath.Join(dest, "repositories.yaml")
	settings.RepositoryCache = filepath.Join(dest, "index-cache")
	settings.RegistryConfig = filepath.Join(dest, "registry.json")
	settings.PluginsDirectory = filepath.Join(dest, "no-plugins")
	options := []registry.ClientOption{registry.ClientOptCredentialsFile(settings.RegistryConfig), registry.ClientOptWriter(io.Discard)}
	if src.Username != "" {
		options = append(options, registry.ClientOptBasicAuth(src.Username, src.Password))
	}
	if src.PlainHTTP {
		options = append(options, registry.ClientOptPlainHTTP())
	}
	registryClient, err := registry.NewClient(options...)
	if err != nil {
		return "", "", err
	}
	pull := action.NewPull(action.WithConfig(&action.Configuration{RegistryClient: registryClient}))
	pull.Settings = settings
	pull.DestDir = dest
	pull.Version = src.Version
	pull.Username, pull.Password = src.Username, src.Password
	pull.PlainHTTP = src.PlainHTTP
	pull.CaFile = src.CAFile
	pull.SetRegistryClient(registryClient)
	ref := src.Name
	if strings.HasPrefix(src.Repository, "oci://") {
		ref = strings.TrimSuffix(src.Repository, "/") + "/" + src.Name
	} else {
		pull.RepoURL = src.Repository
	}
	if _, err := pull.Run(ref); err != nil {
		return "", "", fmt.Errorf("pull chart %s %s: %w", src.Name, src.Version, err)
	}
	archive, err := findArchive(dest)
	if err != nil {
		return "", "", err
	}
	return digestFile(archive)
}

func findArchive(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.tgz"))
	if err != nil || len(matches) != 1 {
		return "", fmt.Errorf("no chart archive in %s", dir)
	}
	return matches[0], nil
}

func digestFile(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(data)
	return path, "sha256:" + hex.EncodeToString(sum[:]), nil
}

// PruneCache removes cached charts in cacheDir that no Pull has used since
// olderThan, holding the lock Pull takes for each, and returns what it
// removed. It keeps going past an entry it cannot remove.
func PruneCache(cacheDir string, olderThan time.Time) ([]string, error) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	removed := []string{}
	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dest := filepath.Join(cacheDir, entry.Name())
		lock, _ := pullLocks.LoadOrStore(dest, &sync.Mutex{})
		lock.(*sync.Mutex).Lock()
		info, err := os.Stat(dest)
		if err == nil && info.ModTime().Before(olderThan) {
			if err := os.RemoveAll(dest); err != nil {
				errs = append(errs, err)
			} else {
				removed = append(removed, dest)
			}
		}
		lock.(*sync.Mutex).Unlock()
	}
	return removed, errors.Join(errs...)
}
