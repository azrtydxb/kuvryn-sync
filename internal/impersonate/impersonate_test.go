package impersonate

import (
	"testing"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestForImpersonatesAndCachesPerServiceAccount(t *testing.T) {
	clients := New(&rest.Config{Host: "https://127.0.0.1:6443"}, client.Options{})

	first, err := clients.For("payments", "deployer")
	if err != nil {
		t.Fatal(err)
	}
	again, err := clients.For("payments", "deployer")
	if err != nil {
		t.Fatal(err)
	}
	other, err := clients.For("search", "deployer")
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatal("same service account built a second client")
	}
	if first == other {
		t.Fatal("different namespaces share a client")
	}
	if got := Username("payments", "deployer"); got != "system:serviceaccount:payments:deployer" {
		t.Fatalf("username = %q", got)
	}
}

func TestForRequiresNamespaceAndName(t *testing.T) {
	clients := New(&rest.Config{Host: "https://127.0.0.1:6443"}, client.Options{})
	for _, tc := range [][2]string{{"", "deployer"}, {"payments", ""}} {
		if _, err := clients.For(tc[0], tc[1]); err == nil {
			t.Fatalf("For(%q, %q) succeeded", tc[0], tc[1])
		}
	}
}
