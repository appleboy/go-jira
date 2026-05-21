package storage

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	// crypto/pbkdf2 is a standard-library package as of Go 1.24; its Key
	// returns ([]byte, error). This module targets Go 1.25 (see go.mod), so no
	// golang.org/x/crypto dependency is needed.
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

// On-disk format for the encrypted token file:
//
//	[4 bytes magic "GJOA"]   // go-jira OAuth Archive
//	[1 byte  version = 1]
//	[16 bytes salt]
//	[12 bytes nonce]
//	[ciphertext + 16-byte GCM auth tag]
//
// The key is derived from the master password with PBKDF2-HMAC-SHA256.
const (
	magic     = "GJOA"
	version   = byte(1)
	saltLen   = 16
	nonceLen  = 12
	keyLen    = 32
	headerLen = len(magic) + 1 + saltLen + nonceLen
)

// iterations is the PBKDF2 work factor. It is a var (not a const) solely so
// tests can lower it; production code never changes it from this default.
var iterations = 600_000

// deriveKey stretches the password into a 32-byte AES key with the given salt.
//
// The password is carried as []byte (not string) on purpose: callers can zero
// it after use, which an immutable string cannot. pbkdf2.Key requires a string,
// so exactly one short-lived copy is made here per derive — unavoidable given
// the stdlib signature, and far better for secret hygiene than holding the
// master password as a string throughout the layer.
func deriveKey(password, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, string(password), salt, iterations, keyLen)
}

// encrypt seals plaintext under a key derived from password, prepending the
// format header (magic, version, salt, nonce).
func encrypt(plaintext, password []byte) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("crypto: read salt: %w", err)
	}
	key, err := deriveKey(password, salt)
	if err != nil {
		return nil, fmt.Errorf("crypto: derive key: %w", err)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: read nonce: %w", err)
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)

	var buf bytes.Buffer
	buf.WriteString(magic)
	buf.WriteByte(version)
	buf.Write(salt)
	buf.Write(nonce)
	buf.Write(ct)
	return buf.Bytes(), nil
}

// decrypt validates the header and opens the ciphertext. A wrong password or
// any tampering fails the GCM auth tag and returns an error.
func decrypt(blob, password []byte) ([]byte, error) {
	if len(blob) < headerLen {
		return nil, errors.New("crypto: blob too short")
	}
	if string(blob[:len(magic)]) != magic {
		return nil, errors.New("crypto: bad magic")
	}
	off := len(magic)
	if blob[off] != version {
		return nil, fmt.Errorf("crypto: unsupported version %d", blob[off])
	}
	off++
	salt := blob[off : off+saltLen]
	off += saltLen
	nonce := blob[off : off+nonceLen]
	off += nonceLen
	ct := blob[off:]

	key, err := deriveKey(password, salt)
	if err != nil {
		return nil, fmt.Errorf("crypto: derive key: %w", err)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return pt, nil
}

// newGCM builds an AES-256-GCM AEAD from a 32-byte key.
func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	return gcm, nil
}
