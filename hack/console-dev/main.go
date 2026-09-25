// Command console-dev serves the ksync console on 127.0.0.1:5174 for the UI
// tests and local development. It starts an envtest API server with RBAC and
// the Kuvryn Sync CRDs, and seeds the design's sample Applications. It runs
// the console without OIDC, so real token sign-in works: a request with a
// token session reads as that token, and any other request is signed in as
// the user "viewer" in group "viewers". A view-only ClusterRole binds both
// that group and the ServiceAccount default/console-viewer.
//
//	go run ./hack/console-dev                          serve until interrupted
//	go run ./hack/console-dev set-health <app> <state> change a seeded Application's health
//	go run ./hack/console-dev token                    print a token for default/console-viewer
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/console"
)

const (
	listen = "127.0.0.1:5174"
	// kubeconfigPath is where the running server writes an admin kubeconfig
	// for set-health; *.kubeconfig is ignored by Git.
	kubeconfigPath = "hack/console-dev/.kubeconfig"
	// appNamespace holds the seeded Applications, Repositories, Revisions
	// and ImagePolicies.
	appNamespace = "default"
	// viewerServiceAccount in appNamespace is bound to the view-only
	// ClusterRole, for token sign-in.
	viewerServiceAccount = "console-viewer"
)

// devAuth is the console's real token-only sign-in, with one addition for
// development: a request without a valid session is signed in as a viewer
// instead of being sent to the login page.
type devAuth struct{ *console.Auth }

func (a devAuth) Identity(r *http.Request) (console.Identity, error) {
	if id, err := a.Auth.Identity(r); err == nil {
		return id, nil
	}
	return console.Identity{Username: "viewer", Groups: []string{"viewers"}, Expiry: time.Now().Add(time.Hour)}, nil
}

func main() {
	var err error
	switch {
	case len(os.Args) > 1 && os.Args[1] == "set-health":
		err = setHealth(os.Args[2:])
	case len(os.Args) > 1 && os.Args[1] == "token":
		err = printToken(os.Args[2:])
	case len(os.Args) > 1:
		err = fmt.Errorf("unknown command %q; use set-health or token, or no argument to serve", os.Args[1])
	default:
		err = serve()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "console-dev:", err)
		os.Exit(1)
	}
}

func scheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = corev1alpha1.AddToScheme(s)
	return s
}

// envtestBinaryDir finds the binaries make setup-envtest installs.
func envtestBinaryDir() string {
	if os.Getenv("KUBEBUILDER_ASSETS") != "" {
		return ""
	}
	entries, err := os.ReadDir(filepath.Join("bin", "k8s"))
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			return filepath.Join("bin", "k8s", e.Name())
		}
	}
	return ""
}

func serve() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: envtestBinaryDir(),
	}
	env.ControlPlane.GetAPIServer().Configure().Set("authorization-mode", "RBAC")
	cfg, err := env.Start()
	if err != nil {
		return fmt.Errorf("start envtest (run make setup-envtest first): %w", err)
	}
	defer func() { _ = env.Stop() }()

	admin, err := env.AddUser(envtest.User{Name: "console-dev-admin", Groups: []string{"system:masters"}}, nil)
	if err != nil {
		return err
	}
	kubeconfig, err := admin.KubeConfig()
	if err != nil {
		return err
	}
	if err := os.WriteFile(kubeconfigPath, kubeconfig, 0o600); err != nil {
		return err
	}
	defer func() { _ = os.Remove(kubeconfigPath) }()

	c, err := client.New(cfg, client.Options{Scheme: scheme()})
	if err != nil {
		return err
	}
	if err := seed(ctx, c); err != nil {
		return fmt.Errorf("seed: %w", err)
	}

	consoleCfg := console.Config{
		Listen:          listen,
		ClusterName:     "prod-eu-1",
		DocsURL:         "https://github.com/azrtydxb/kuvryn-sync/tree/main/docs",
		StatusURL:       "https://github.com/azrtydxb/kuvryn-sync/actions",
		InsecureCookies: true,
	}
	srv, err := console.NewServer(consoleCfg, cfg)
	if err != nil {
		return err
	}
	auth, err := console.NewAuth(ctx, consoleCfg, cfg)
	if err != nil {
		return err
	}
	srv.UseAuthenticator(devAuth{auth})
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdown)
	}()
	fmt.Println("ready: http://" + listen)
	if err := httpSrv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// adminConfig connects with the kubeconfig the running server wrote.
func adminConfig() (*rest.Config, error) {
	raw, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("read %s (is console-dev running?): %w", kubeconfigPath, err)
	}
	return clientcmd.RESTConfigFromKubeConfig(raw)
}

// printToken prints a one-hour token for the seeded ServiceAccount
// default/console-viewer, minted with a TokenRequest.
func printToken(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: console-dev token")
	}
	cfg, err := adminConfig()
	if err != nil {
		return err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	seconds := int64(3600)
	tr, err := cs.CoreV1().ServiceAccounts(appNamespace).CreateToken(context.Background(), viewerServiceAccount,
		&authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &seconds}}, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	fmt.Println(tr.Status.Token)
	return nil
}

// setHealth changes a seeded Application's health through the kubeconfig the
// running server wrote.
func setHealth(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: console-dev set-health <application> <Healthy|Progressing|Degraded|Suspended|Unknown>")
	}
	cfg, err := adminConfig()
	if err != nil {
		return err
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme()})
	if err != nil {
		return err
	}
	ctx := context.Background()
	app := &corev1alpha1.Application{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: appNamespace, Name: args[0]}, app); err != nil {
		return err
	}
	patch := client.MergeFrom(app.DeepCopy())
	app.Status.Health.State = corev1alpha1.HealthState(args[1])
	return c.Status().Patch(ctx, app, patch)
}
