package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/signal"
	"syscall"

	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/azrtydxb/kuvryn-sync/internal/console"
)

// runConsole serves the read-only web console until interrupted.
func runConsole(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConsoleFlags(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	restConfig, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("load cluster configuration: %w", err)
	}
	srv, err := console.NewServer(cfg, restConfig)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	_, _ = fmt.Fprintf(stdout, "ksync console listening on %s\n", cfg.Listen)
	return srv.Run(ctx)
}

func parseConsoleFlags(args []string, stderr io.Writer) (console.Config, error) {
	var cfg console.Config
	fs := flag.NewFlagSet("ksync console", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg.BindFlags(fs)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "Usage: ksync console [flags]\n\nServe the read-only web console.\n\nFlags:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() > 0 {
		return cfg, fmt.Errorf("ksync console takes no arguments, got %q", fs.Args())
	}
	return cfg, nil
}
