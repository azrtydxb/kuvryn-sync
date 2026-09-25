package decrypt

import (
	"strings"
	"testing"

	agelib "filippo.io/age"

	"github.com/azrtydxb/kuvryn-sync/internal/decrypt/decrypttest"
)

const plainSecret = `apiVersion: v1
kind: Secret
metadata:
  name: db
stringData:
  password: hunter2
`

func identity(t *testing.T) *agelib.X25519Identity {
	t.Helper()
	id, err := agelib.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestDecryptsWithTheMatchingAgeKey(t *testing.T) {
	key := identity(t)
	encrypted := decrypttest.Encrypt(t, key.Recipient().String(), plainSecret)
	if !Encrypted(encrypted) || strings.Contains(string(encrypted), "hunter2") {
		t.Fatalf("fixture is not encrypted:\n%s", encrypted)
	}
	d, err := New(key.String())
	if err != nil {
		t.Fatal(err)
	}
	out, err := d.File("secret.yaml", encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "password: hunter2") || strings.Contains(string(out), "sops:") {
		t.Fatalf("decrypted:\n%s", out)
	}
}

func TestRejectsWrongKeyTamperingAndMissingDecryption(t *testing.T) {
	key := identity(t)
	encrypted := decrypttest.Encrypt(t, key.Recipient().String(), plainSecret)

	other, err := New(identity(t).String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.File("secret.yaml", encrypted); err == nil {
		t.Fatal("decrypted with a key that is not a recipient")
	}

	right, err := New(key.String())
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(encrypted), "name: db", "name: other", 1)
	if _, err := right.File("secret.yaml", []byte(tampered)); err == nil || !strings.Contains(err.Error(), "MAC") {
		t.Fatalf("tampered file: err = %v", err)
	}

	var none *Decryptor
	if _, err := none.File("secret.yaml", encrypted); err == nil || !strings.Contains(err.Error(), "spec.decryption") {
		t.Fatalf("no decryption configured: err = %v", err)
	}
	if out, err := none.File("plain.yaml", []byte(plainSecret)); err != nil || string(out) != plainSecret {
		t.Fatalf("plain file changed: %v", err)
	}
}
