package oauth

import (
	"regexp"
	"testing"
)

// verifierCharset matches the RFC 7636 unreserved character set.
var verifierCharset = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)

func TestNewVerifier(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		v := NewVerifier()
		if l := len(v); l < 43 || l > 128 {
			t.Errorf("verifier length %d outside RFC 7636 range 43..128", l)
		}
		if !verifierCharset.MatchString(v) {
			t.Errorf("verifier %q contains disallowed characters", v)
		}
		if seen[v] {
			t.Errorf("verifier %q repeated — not random", v)
		}
		seen[v] = true
	}
}

func TestNewState(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		s, err := NewState()
		if err != nil {
			t.Fatalf("NewState: %v", err)
		}
		if s == "" {
			t.Fatal("NewState returned empty string")
		}
		if seen[s] {
			t.Errorf("state %q repeated — not random", s)
		}
		seen[s] = true
	}
}
