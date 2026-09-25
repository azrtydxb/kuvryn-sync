// Package console serves the read-only Kuvryn Sync web console: sign-in with a
// Kubernetes token or, optionally, OIDC, a JSON API that reads the cluster as
// the signed-in user, and the embedded single-page application.
package console

import (
	"errors"
	"flag"
	"fmt"
	"net"
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
	SSOName          string
	Connectors       []string
	DocsURL          string
	StatusURL        string
	InsecureCookies  bool
}

// BindFlags registers every console flag on fs, writing parsed values to c.
func (c *Config) BindFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.Listen, "listen", ":8080", "address the console listens on")
	fs.StringVar(&c.IssuerURL, "oidc-issuer-url", "", "OIDC issuer URL, for example https://dex.example.com; optional, and set together with --oidc-client-id")
	fs.StringVar(&c.ClientID, "oidc-client-id", "", "OIDC client ID; optional, and set together with --oidc-issuer-url")
	fs.StringVar(&c.ClientSecretFile, "oidc-client-secret-file", "", "file holding the OIDC client secret")
	fs.StringVar(&c.RedirectURL, "redirect-url", "", "OIDC redirect URL, ending in /auth/callback")
	fs.StringVar(&c.UsernameClaim, "username-claim", "email", "ID token claim used as the Kubernetes username")
	fs.StringVar(&c.GroupsClaim, "groups-claim", "groups", "ID token claim used as the Kubernetes groups")
	fs.StringVar(&c.UsernamePrefix, "username-prefix", "", "prefix added to the username before impersonating it")
	fs.StringVar(&c.GroupsPrefix, "groups-prefix", "", "prefix added to each group before impersonating it")
	fs.StringVar(&c.SessionKeyFile, "session-key-file", "", "file holding the 32-byte session cookie encryption key; without it, a random key is kept in memory, so sessions end on restart and are not shared between replicas")
	fs.StringVar(&c.ClusterName, "cluster-name", "cluster", "cluster name shown in the console")
	fs.StringVar(&c.SSOName, "sso-name", "", `identity provider named on the login page's "Sign in with" button, for example Dex`)
	fs.Func("connectors", "comma-separated Dex connectors offered at sign-in: github, gitlab, local", func(v string) error {
		c.Connectors = splitList(v)
		return nil
	})
	fs.StringVar(&c.DocsURL, "docs-url", "", "documentation link shown in the console")
	fs.StringVar(&c.StatusURL, "status-url", "", "status page link shown in the console")
	fs.BoolVar(&c.InsecureCookies, "insecure-cookies", false, "allow cookies over plain HTTP, for local development only; --redirect-url (with OIDC) or --listen (without) must then be on localhost, 127.0.0.1 or [::1]")
}

// OIDCEnabled reports whether OIDC sign-in is configured. Token sign-in is
// always available; OIDC is offered next to it only with an issuer and a
// client ID.
func (c Config) OIDCEnabled() bool { return c.IssuerURL != "" || c.ClientID != "" }

// Validate refuses settings the console cannot run with: only one of the two
// OIDC flags, OIDC without a redirect URL or username claim, and insecure
// cookies anywhere but on a loopback address.
func (c Config) Validate() error {
	if !c.OIDCEnabled() {
		if c.InsecureCookies && !isLoopbackHost(c.Listen) {
			return fmt.Errorf("console: --insecure-cookies is for local development only; --listen %q must be on localhost, 127.0.0.1 or [::1]", c.Listen)
		}
		return nil
	}
	if c.IssuerURL == "" || c.ClientID == "" {
		return errors.New("console: --oidc-issuer-url and --oidc-client-id must be set together, or both left out for token sign-in only")
	}
	if c.RedirectURL == "" {
		return errors.New("console: OIDC sign-in needs --redirect-url, ending in /auth/callback")
	}
	if c.UsernameClaim == "" {
		return errors.New("console: --username-claim must not be empty")
	}
	if c.InsecureCookies && !isLoopbackURL(c.RedirectURL) {
		return fmt.Errorf("console: --insecure-cookies is for local development only; --redirect-url %q must be on localhost, 127.0.0.1 or [::1]", c.RedirectURL)
	}
	return nil
}

// isLoopbackHost reports whether a host:port listen address names localhost
// or a loopback address. An empty host listens everywhere, so it is not.
func isLoopbackHost(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
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
