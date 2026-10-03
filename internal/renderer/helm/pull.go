package helm

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/registry"
	repo "helm.sh/helm/v4/pkg/repo/v1"
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

// pullTimeout bounds each request to a chart registry, so a registry that
// accepts a connection and never answers cannot hold a reconcile worker, and
// the chart's pull lock, forever. Helm's own HTTP repository getter already
// times out after two minutes.
var pullTimeout = 2 * time.Minute

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
	settings.ContentCache = filepath.Join(dest, "content-cache")
	settings.RegistryConfig = filepath.Join(dest, "registry.json")
	settings.PluginsDirectory = filepath.Join(dest, "no-plugins")
	options := []registry.ClientOption{
		registry.ClientOptCredentialsFile(settings.RegistryConfig),
		registry.ClientOptWriter(io.Discard),
		registry.ClientOptHTTPClient(&http.Client{Timeout: pullTimeout, Transport: registry.NewTransport(false)}),
	}
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
	var ref string
	if strings.HasPrefix(src.Repository, "oci://") {
		ref = strings.TrimSuffix(src.Repository, "/") + "/" + src.Name
	} else {
		// Helm's pull.RepoURL lookup writes the repository index to the
		// default cache, so look the chart up here and pull its URL.
		ref, err = findChartURL(settings, src)
		if err != nil {
			return "", "", fmt.Errorf("pull chart %s %s: %w", src.Name, src.Version, err)
		}
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

// findChartURL resolves a chart in an https Helm repository to its download
// URL, like Helm's repo.FindChartInRepoURL but with the index written to
// settings.RepositoryCache instead of Helm's default cache, which need not be
// writable.
func findChartURL(settings *cli.EnvSettings, src ChartSource) (string, error) {
	chartRepo, err := repo.NewChartRepository(&repo.Entry{
		Name:     "chart",
		URL:      src.Repository,
		Username: src.Username,
		Password: src.Password,
		CAFile:   src.CAFile,
	}, getter.All(settings))
	if err != nil {
		return "", err
	}
	chartRepo.CachePath = settings.RepositoryCache
	// The index is only needed for this lookup, and can be large.
	defer func() { _ = os.RemoveAll(settings.RepositoryCache) }()
	indexPath, err := chartRepo.DownloadIndexFile()
	if err != nil {
		return "", fmt.Errorf("looks like %q is not a valid chart repository or cannot be reached: %w", src.Repository, err)
	}
	index, err := repo.LoadIndexFile(indexPath)
	if err != nil {
		return "", err
	}
	version, err := index.Get(src.Name, src.Version)
	if err != nil {
		return "", repo.ChartNotFoundError{Chart: fmt.Sprintf("chart %q version %q", src.Name, src.Version), RepoURL: src.Repository}
	}
	if len(version.URLs) == 0 {
		return "", fmt.Errorf("chart %q version %q has no downloadable URLs", src.Name, src.Version)
	}
	return repo.ResolveReferenceURL(src.Repository, version.URLs[0])
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
		switch {
		case errors.Is(err, os.ErrNotExist):
			// Removed since it was listed; nothing to do.
		case err != nil:
			errs = append(errs, err)
		case info.ModTime().Before(olderThan):
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
