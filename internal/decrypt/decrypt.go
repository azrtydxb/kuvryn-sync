// Package decrypt decrypts SOPS-encrypted manifests with age keys, in memory,
// without touching process-wide sops configuration.
package decrypt

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/getsops/sops/v3/aes"
	"github.com/getsops/sops/v3/age"
	"github.com/getsops/sops/v3/cmd/sops/common"
	"github.com/getsops/sops/v3/cmd/sops/formats"
	"github.com/getsops/sops/v3/config"
	"github.com/getsops/sops/v3/keyservice"
	sigsyaml "sigs.k8s.io/yaml"
)

// Decryptor decrypts SOPS files for which one of its age identities is a recipient.
type Decryptor struct {
	identities age.ParsedIdentities
}

// New parses age identities (AGE-SECRET-KEY-1... lines).
func New(ageKeys ...string) (*Decryptor, error) {
	d := &Decryptor{}
	if err := d.identities.Import(ageKeys...); err != nil {
		return nil, err
	}
	return d, nil
}

// Encrypted reports whether a YAML or JSON document carries SOPS metadata.
func Encrypted(data []byte) bool {
	var doc map[string]any
	if err := sigsyaml.Unmarshal(data, &doc); err != nil {
		return false
	}
	metadata, ok := doc["sops"].(map[string]any)
	return ok && metadata["mac"] != nil
}

// File returns data decrypted when it is a SOPS file and unchanged otherwise.
// A nil Decryptor refuses encrypted files, so they are never applied as
// ciphertext.
func (d *Decryptor) File(path string, data []byte) ([]byte, error) {
	if !Encrypted(data) {
		return data, nil
	}
	if d == nil {
		return nil, fmt.Errorf("%s is SOPS-encrypted but the Application has no spec.decryption", path)
	}
	format := formats.Yaml
	if strings.EqualFold(filepath.Ext(path), ".json") {
		format = formats.Json
	}
	store := common.StoreForFormat(format, config.NewStoresConfig())
	tree, err := store.LoadEncryptedFile(data)
	if err != nil {
		return nil, fmt.Errorf("%s: load SOPS file: %w", path, err)
	}
	if _, err := common.DecryptTree(common.DecryptTreeOpts{
		Tree:        &tree,
		KeyServices: []keyservice.KeyServiceClient{keyservice.NewCustomLocalClient(ageOnly{identities: d.identities})},
		Cipher:      aes.NewCipher(),
	}); err != nil {
		return nil, fmt.Errorf("%s: decrypt SOPS file: %w", path, err)
	}
	out, err := store.EmitPlainFile(tree.Branches)
	if err != nil {
		return nil, fmt.Errorf("%s: emit decrypted SOPS file: %w", path, err)
	}
	return out, nil
}

// ageOnly is an in-process key service that decrypts SOPS data keys with the
// given age identities and refuses every other key type.
type ageOnly struct {
	keyservice.UnimplementedKeyServiceServer
	identities age.ParsedIdentities
}

func (s ageOnly) Decrypt(_ context.Context, req *keyservice.DecryptRequest) (*keyservice.DecryptResponse, error) {
	key, ok := req.Key.KeyType.(*keyservice.Key_AgeKey)
	if !ok {
		return nil, fmt.Errorf("only age keys are supported")
	}
	master := &age.MasterKey{Recipient: key.AgeKey.Recipient, EncryptedKey: string(req.Ciphertext)}
	s.identities.ApplyToMasterKey(master)
	plaintext, err := master.Decrypt()
	if err != nil {
		return nil, err
	}
	return &keyservice.DecryptResponse{Plaintext: plaintext}, nil
}
