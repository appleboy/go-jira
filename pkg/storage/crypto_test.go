package storage

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	plaintext := []byte(`{"tokens":{"abc":{"access_token":"xyz"}}}`)
	password := []byte("correct horse battery staple")

	blob, err := encrypt(plaintext, password)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Contains(blob, plaintext) {
		t.Error("ciphertext contains plaintext — not encrypted")
	}
	if string(blob[:len(magic)]) != magic {
		t.Errorf("missing magic header")
	}

	got, err := decrypt(blob, password)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("round-trip mismatch: got %q", got)
	}
}

func TestDecryptWrongPassword(t *testing.T) {
	blob, err := encrypt([]byte("secret"), []byte("right"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := decrypt(blob, []byte("wrong")); err == nil {
		t.Error("decrypt with wrong password should fail")
	}
}

func TestDecryptTamperDetected(t *testing.T) {
	password := []byte("pw")
	blob, err := encrypt([]byte("secret payload"), password)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	// Flip a bit in the ciphertext (last byte, inside the GCM-protected area).
	tampered := bytes.Clone(blob)
	tampered[len(tampered)-1] ^= 0x01
	if _, err := decrypt(tampered, password); err == nil {
		t.Error("decrypt of tampered ciphertext should fail the auth tag")
	}
}

func TestDecryptMalformed(t *testing.T) {
	password := []byte("pw")
	tests := map[string][]byte{
		"too short":   []byte("GJOA"),
		"bad magic":   append([]byte("XXXX"), make([]byte, headerLen)...),
		"bad version": append([]byte(magic), append([]byte{9}, make([]byte, headerLen)...)...),
	}
	for name, blob := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decrypt(blob, password); err == nil {
				t.Error("expected error for malformed blob")
			}
		})
	}
}
