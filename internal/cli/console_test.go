package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestConsoleHelpListsEveryFlag(t *testing.T) {
	var out, errb bytes.Buffer
	if _, code := Run(context.Background(), []string{"console", "--help"}, &out, &errb); code != 0 {
		t.Fatalf("console --help exit %d: %s", code, errb.String())
	}
	for _, flag := range []string{
		"-listen", "-oidc-issuer-url", "-oidc-client-id", "-oidc-client-secret-file", "-redirect-url",
		"-username-claim", "-groups-claim", "-username-prefix", "-groups-prefix", "-session-key-file",
		"-cluster-name", "-sso-name", "-connectors", "-docs-url", "-status-url", "-insecure-cookies",
	} {
		if !strings.Contains(errb.String(), flag+" ") && !strings.Contains(errb.String(), flag+"\n") {
			t.Errorf("console --help lacks %s:\n%s", flag, errb.String())
		}
	}
	cfg, err := parseConsoleFlags([]string{"--connectors", "github, gitlab,,local"}, &errb)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cfg.Connectors, ",") != "github,gitlab,local" || cfg.UsernameClaim != "email" || cfg.GroupsClaim != "groups" || cfg.Listen != ":8080" || cfg.ClusterName != "cluster" {
		t.Fatalf("defaults or connectors wrong: %+v", cfg)
	}
}
