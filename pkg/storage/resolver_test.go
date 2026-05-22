package storage

import (
	"errors"
	"path/filepath"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

func TestResolvePrefersKeyring(t *testing.T) {
	keyring.MockInit()
	s, err := Resolve(ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if s.Backend() != "keyring" {
		t.Errorf("Backend() = %q, want keyring", s.Backend())
	}
}

func TestResolveFallsBackToFile(t *testing.T) {
	keyring.MockInitWithError(errors.New("no keyring here"))
	path := filepath.Join(t.TempDir(), "tokens.enc")

	s, err := Resolve(ResolveOptions{Password: "pw", FilePath: path})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if s.Backend() != "file" {
		t.Errorf("Backend() = %q, want file", s.Backend())
	}
}

func TestResolveFileWithoutPasswordErrors(t *testing.T) {
	keyring.MockInitWithError(errors.New("no keyring here"))
	if _, err := Resolve(ResolveOptions{}); err == nil {
		t.Error("expected error when keyring unavailable and no password")
	}
}

func TestResolveFileUsesDefaultPath(t *testing.T) {
	keyring.MockInitWithError(errors.New("no keyring here"))
	// No FilePath given: Resolve must derive the default location. It only
	// constructs the store (no file is written), so this is side-effect free.
	s, err := Resolve(ResolveOptions{Password: "pw"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	fs, ok := s.(*FileStore)
	if !ok {
		t.Fatalf("expected *FileStore, got %T", s)
	}
	if filepath.Base(fs.Path) != "tokens.enc" {
		t.Errorf("default path = %q, want it to end in tokens.enc", fs.Path)
	}
}

func TestResolveForceFile(t *testing.T) {
	keyring.MockInit() // keyring works, but ForceFile should bypass it
	path := filepath.Join(t.TempDir(), "tokens.enc")

	s, err := Resolve(ResolveOptions{ForceFile: true, Password: "pw", FilePath: path})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if s.Backend() != "file" {
		t.Errorf("Backend() = %q, want file", s.Backend())
	}
}
