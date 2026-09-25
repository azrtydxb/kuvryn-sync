package console

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// SessionKeySize is the length of the AES-256 session key.
const SessionKeySize = 32

var errSealedValue = errors.New("console: sealed value is invalid")

// seal encrypts v as JSON with AES-256-GCM under a random nonce, returning
// base64url(nonce || ciphertext).
func seal(key []byte, v any) (string, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return "", err
	}
	plain, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(aead.Seal(nonce, nonce, plain, nil)), nil
}

// open decrypts a value sealed by seal into v. It fails for any value that was
// not sealed under key, including every value after a key rotation.
func open(key []byte, s string, v any) error {
	aead, err := newAEAD(key)
	if err != nil {
		return err
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(raw) < aead.NonceSize() {
		return errSealedValue
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], nil)
	if err != nil {
		return errSealedValue
	}
	if err := json.Unmarshal(plain, v); err != nil {
		return errSealedValue
	}
	return nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != SessionKeySize {
		return nil, fmt.Errorf("console: session key is %d bytes, want %d", len(key), SessionKeySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Secret holds a bearer token. It prints and marshals as "[redacted]", so an
// Identity that reaches a log line or a response never shows the token.
type Secret string

const redactedSecret = "[redacted]"

// String implements fmt.Stringer.
func (Secret) String() string { return redactedSecret }

// GoString implements fmt.GoStringer, for %#v.
func (Secret) GoString() string { return redactedSecret }

// MarshalJSON implements json.Marshaler.
func (Secret) MarshalJSON() ([]byte, error) { return json.Marshal(redactedSecret) }

// MarshalText implements encoding.TextMarshaler.
func (Secret) MarshalText() ([]byte, error) { return []byte(redactedSecret), nil }

// Reveal returns the token, for the one place that sends it: the
// Authorization header of the session's own requests.
func (s Secret) Reveal() string { return string(s) }
