// Package decrypttest creates SOPS fixtures for tests.
package decrypttest

import (
	agelib "filippo.io/age"
	"github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	sopsage "github.com/getsops/sops/v3/age"
	"github.com/getsops/sops/v3/cmd/sops/common"
	"github.com/getsops/sops/v3/cmd/sops/formats"
	"github.com/getsops/sops/v3/config"
)

// T is the part of testing.T (or GinkgoT) the helpers need.
type T interface {
	Helper()
	Fatal(args ...any)
}

// Identity generates an age key pair.
func Identity(t T) *agelib.X25519Identity {
	t.Helper()
	id, err := agelib.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Encrypt returns plain encrypted with SOPS to the recipient, for tests.
func Encrypt(t T, recipient string, plain string) []byte {
	t.Helper()
	store := common.StoreForFormat(formats.Yaml, config.NewStoresConfig())
	branches, err := store.LoadPlainFile([]byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	key, err := sopsage.MasterKeyFromRecipient(recipient)
	if err != nil {
		t.Fatal(err)
	}
	tree := sops.Tree{Branches: branches, Metadata: sops.Metadata{KeyGroups: []sops.KeyGroup{{key}}, EncryptedRegex: "^(data|stringData)$"}}
	dataKey, errs := tree.GenerateDataKey()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if err := common.EncryptTree(common.EncryptTreeOpts{DataKey: dataKey, Tree: &tree, Cipher: aes.NewCipher()}); err != nil {
		t.Fatal(err)
	}
	out, err := store.EmitEncryptedFile(tree)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
