// Package console serves the read-only Kuvryn Sync web console: OIDC sign-in,
// a JSON API that reads the cluster as the signed-in user, and the embedded
// single-page application.
package console

import (
	"flag"
	"strings"
)

// Config holds the ksync console settings, bound from command-line flags.
type Config struct {
	Listen           string
	IssuerURL        string
	ClientID         string
	ClientSecretFile string
	RedirectURL      string
	UsernameClaim    string
	GroupsClaim      string
	UsernamePrefix   string
	GroupsPrefix     string
	SessionKeyFile   string
	ClusterName      string
	Connectors       []string
	DocsURL          string
	StatusURL        string
	InsecureCookies  bool
}

// BindFlags registers every console flag on fs, writing parsed values to c.
func (c *Config) BindFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.Listen, "listen", ":8080", "address the console listens on")
	fs.StringVar(&c.IssuerURL, "oidc-issuer-url", "", "OIDC issuer URL, for example https://dex.example.com")
	fs.StringVar(&c.ClientID, "oidc-client-id", "", "OIDC client ID")
	fs.StringVar(&c.ClientSecretFile, "oidc-client-secret-file", "", "file holding the OIDC client secret")
	fs.StringVar(&c.RedirectURL, "redirect-url", "", "OIDC redirect URL, ending in /auth/callback")
	fs.StringVar(&c.UsernameClaim, "username-claim", "email", "ID token claim used as the Kubernetes username")
	fs.StringVar(&c.GroupsClaim, "groups-claim", "groups", "ID token claim used as the Kubernetes groups")
	fs.StringVar(&c.UsernamePrefix, "username-prefix", "", "prefix added to the username before impersonating it")
	fs.StringVar(&c.GroupsPrefix, "groups-prefix", "", "prefix added to each group before impersonating it")
	fs.StringVar(&c.SessionKeyFile, "session-key-file", "", "file holding the 32-byte session cookie encryption key")
	fs.StringVar(&c.ClusterName, "cluster-name", "cluster", "cluster name shown in the console")
	fs.Func("connectors", "comma-separated Dex connectors offered at sign-in: github, gitlab, local", func(v string) error {
		c.Connectors = splitList(v)
		return nil
	})
	fs.StringVar(&c.DocsURL, "docs-url", "", "documentation link shown in the console")
	fs.StringVar(&c.StatusURL, "status-url", "", "status page link shown in the console")
	fs.BoolVar(&c.InsecureCookies, "insecure-cookies", false, "allow cookies over plain HTTP, for local development only")
}

func splitList(v string) []string {
	out := []string{}
	for item := range strings.SplitSeq(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
