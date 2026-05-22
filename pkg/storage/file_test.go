package storage

import (
	"errors"
	"path/filepath"
	"testing"
)

func newFileStore(t *testing.T) *FileStore {
	t.Helper()
	return &FileStore{
		Path:     filepath.Join(t.TempDir(), "tokens.enc"),
		Password: []byte("master-pw"),
	}
}

// TestFileStoreEmptyPassword verifies the file backend refuses to operate with
// an empty master password rather than deriving a key from it.
func TestFileStoreEmptyPassword(t *testing.T) {
	s := &FileStore{Path: filepath.Join(t.TempDir(), "tokens.enc")}
	key := MakeKey("https://jira.example.com", "client-abc")

	if err := s.Save(key, sampleToken()); !errors.Is(err, errEmptyPassword) {
		t.Errorf("Save error = %v, want errEmptyPassword", err)
	}
	if _, err := s.Load(key); !errors.Is(err, errEmptyPassword) {
		t.Errorf("Load error = %v, want errEmptyPassword", err)
	}
	if err := s.Delete(key); !errors.Is(err, errEmptyPassword) {
		t.Errorf("Delete error = %v, want errEmptyPassword", err)
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	s := newFileStore(t)
	if s.Backend() != "file" {
		t.Errorf("Backend() = %q, want file", s.Backend())
	}

	key := MakeKey("https://jira.example.com", "client-abc")
	tok := sampleToken()

	if err := s.Save(key, tok); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load(key)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	assertTokenEqual(t, got, tok)
}

func TestFileStoreMultipleKeysCoexist(t *testing.T) {
	s := newFileStore(t)
	k1 := MakeKey("https://a.example.com", "client")
	k2 := MakeKey("https://b.example.com", "client")

	t1 := sampleToken()
	t1.AccessToken = "token-a"
	t2 := sampleToken()
	t2.AccessToken = "token-b"

	if err := s.Save(k1, t1); err != nil {
		t.Fatalf("Save k1: %v", err)
	}
	if err := s.Save(k2, t2); err != nil {
		t.Fatalf("Save k2: %v", err)
	}

	got1, err := s.Load(k1)
	if err != nil {
		t.Fatalf("Load k1: %v", err)
	}
	got2, err := s.Load(k2)
	if err != nil {
		t.Fatalf("Load k2: %v", err)
	}
	if got1.AccessToken != "token-a" || got2.AccessToken != "token-b" {
		t.Errorf("keys clobbered each other: %q / %q", got1.AccessToken, got2.AccessToken)
	}
}

func TestFileStoreLoadMissing(t *testing.T) {
	s := newFileStore(t)
	if _, err := s.Load("missing"); err != ErrTokenNotFound {
		t.Errorf("Load missing = %v, want ErrTokenNotFound", err)
	}
}

func TestFileStoreDelete(t *testing.T) {
	s := newFileStore(t)
	key := MakeKey("https://jira.example.com", "client-abc")

	if err := s.Save(key, sampleToken()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Delete(key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Load(key); err != ErrTokenNotFound {
		t.Errorf("Load after delete = %v, want ErrTokenNotFound", err)
	}
	// Deleting again is a no-op.
	if err := s.Delete(key); err != nil {
		t.Errorf("second Delete = %v, want nil", err)
	}
}

func TestFileStoreWrongPasswordFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.enc")
	key := MakeKey("https://jira.example.com", "client-abc")

	good := &FileStore{Path: path, Password: []byte("right-pw")}
	if err := good.Save(key, sampleToken()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	bad := &FileStore{Path: path, Password: []byte("wrong-pw")}
	if _, err := bad.Load(key); err == nil {
		t.Error("Load with wrong password should fail")
	}
}
