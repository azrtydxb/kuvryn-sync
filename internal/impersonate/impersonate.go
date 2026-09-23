// Package impersonate builds Kubernetes clients that act as an Application's
// service account, so tenant RBAC decides what Solder may change.
package impersonate

import (
	"fmt"
	"sync"

	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Username returns the Kubernetes user name of a service account.
func Username(namespace, serviceAccount string) string {
	return serviceaccount.MakeUsername(namespace, serviceAccount)
}

// Clients builds and caches one uncached client per impersonated service account.
type Clients struct {
	config  *rest.Config
	options client.Options

	mu      sync.Mutex
	clients map[string]client.Client
}

// New returns a Clients that derives impersonating clients from config.
func New(config *rest.Config, options client.Options) *Clients {
	return &Clients{config: config, options: options, clients: map[string]client.Client{}}
}

// For returns a client whose requests are authorized as the service account.
func (c *Clients) For(namespace, serviceAccount string) (client.Client, error) {
	if namespace == "" || serviceAccount == "" {
		return nil, fmt.Errorf("service account namespace and name are required")
	}
	user := Username(namespace, serviceAccount)
	c.mu.Lock()
	defer c.mu.Unlock()
	if cached, ok := c.clients[user]; ok {
		return cached, nil
	}
	config := rest.CopyConfig(c.config)
	config.Impersonate = rest.ImpersonationConfig{UserName: user}
	options := c.options
	options.HTTPClient = nil
	impersonated, err := client.New(config, options)
	if err != nil {
		return nil, err
	}
	// debt: one client per service account is kept for the process lifetime;
	// revisit if tenants churn through many short-lived service accounts.
	c.clients[user] = impersonated
	return impersonated, nil
}
