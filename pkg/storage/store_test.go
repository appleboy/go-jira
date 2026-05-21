package storage

import (
	"os"
	"testing"
)

// TestMain lowers the PBKDF2 work factor for the whole package test run so the
// many encrypt/decrypt round-trips stay fast. Production keeps the 600k
// default; the round-trip and tamper properties hold at any iteration count.
func TestMain(m *testing.M) {
	iterations = 4096
	os.Exit(m.Run())
}

func TestMakeKey(t *testing.T) {
	k1 := MakeKey("https://jira.example.com", "client-a")
	k2 := MakeKey("https://jira.example.com", "client-b")
	k3 := MakeKey("https://other.example.com", "client-a")

	if len(k1) != 64 {
		t.Errorf("key length = %d, want 64", len(k1))
	}
	if k1 == k2 || k1 == k3 || k2 == k3 {
		t.Error("keys for distinct (baseURL, clientID) pairs must differ")
	}
	if MakeKey("https://jira.example.com", "client-a") != k1 {
		t.Error("MakeKey must be deterministic")
	}
}
