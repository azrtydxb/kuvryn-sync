// Command console-dev serves the ksync console on :5174 for the UI tests,
// with a stub sign-in that always returns a viewer.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/client-go/rest"

	"github.com/azrtydxb/kuvryn-sync/internal/console"
)

// viewerAuth signs every request in as a viewer.
type viewerAuth struct{}

func (viewerAuth) Identity(*http.Request) (console.Identity, error) {
	return console.Identity{Username: "viewer", Groups: []string{"viewers"}}, nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "console-dev:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cfg := console.Config{
		Listen:          "127.0.0.1:5174",
		ClusterName:     "prod-eu-1",
		SSOName:         "Dex",
		Connectors:      []string{"github"},
		DocsURL:         "https://github.com/azrtydxb/kuvryn-sync/tree/main/docs",
		StatusURL:       "https://github.com/azrtydxb/kuvryn-sync/actions",
		InsecureCookies: true,
	}
	srv, err := console.NewServer(cfg, &rest.Config{Host: "https://127.0.0.1:1"})
	if err != nil {
		return err
	}
	srv.UseAuthenticator(viewerAuth{})
	fmt.Println("ready")
	return srv.Run(ctx)
}
