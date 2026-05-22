package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FileStore persists tokens in a single AES-256-GCM encrypted file. All keys
// share one file, which is decrypted, mutated, and re-encrypted on each write.
//
// Concurrency: a write is read→decrypt→mutate→encrypt→atomic-rename, with no
// cross-process lock. Within a process this is fine (the CLI does one operation
// at a time); across processes, two concurrent writes to the same file can race
// so the last rename wins and drops the other's update. This is acceptable for
// the intended use — the file backend is the fallback when no OS keyring is
// available (e.g. a headless CI box), where concurrent token writes to the same
// file are not expected; interactive use prefers the keyring.
type FileStore struct {
	Path     string // e.g. ~/.config/go-jira/tokens.enc
	Password []byte // master password (from JIRA_MASTER_PASSWORD via ResolveOptions)
}

// fileContents is the decrypted JSON document: key -> token.
type fileContents struct {
	Tokens map[string]*StoredToken `json:"tokens"`
}

// errEmptyPassword guards the file backend against a zero-length master
// password. Resolve never selects the file backend without a password, but
// FileStore is exported and can be built directly; deriving a key from an empty
// password would silently "encrypt" tokens under a guessable key.
var errEmptyPassword = errors.New("storage: file backend requires a non-empty master password")

func (s *FileStore) load() (*fileContents, error) {
	if len(s.Password) == 0 {
		return nil, errEmptyPassword
	}
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &fileContents{Tokens: map[string]*StoredToken{}}, nil
		}
		return nil, fmt.Errorf("file read: %w", err)
	}
	pt, err := decrypt(raw, s.Password)
	if err != nil {
		return nil, err
	}
	var fc fileContents
	if err := json.Unmarshal(pt, &fc); err != nil {
		return nil, fmt.Errorf("file unmarshal: %w", err)
	}
	if fc.Tokens == nil {
		fc.Tokens = map[string]*StoredToken{}
	}
	return &fc, nil
}

func (s *FileStore) save(fc *fileContents) error {
	if len(s.Password) == 0 {
		return errEmptyPassword
	}
	pt, err := json.Marshal(fc) // #nosec G117 -- persisting the token is the point
	if err != nil {
		return fmt.Errorf("file marshal: %w", err)
	}
	blob, err := encrypt(pt, s.Password)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("file mkdir: %w", err)
	}
	// Write to a temp file in the same dir, fsync it, and rename into place, so a
	// crash or interruption mid-write can't leave a truncated/corrupt tokens.enc
	// that would break every future load. The Sync flushes the bytes before the
	// rename; the directory entry is not fsynced, so this is an atomic,
	// data-flushed replace rather than a full power-loss guarantee.
	tmp, err := os.CreateTemp(dir, ".tokens-*.tmp")
	if err != nil {
		return fmt.Errorf("file temp create: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("file temp chmod: %w", err)
	}
	if _, err := tmp.Write(blob); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("file temp write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("file temp sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("file temp close: %w", err)
	}
	if err := os.Rename(tmpName, s.Path); err != nil {
		return fmt.Errorf("file rename: %w", err)
	}
	return nil
}

// Save upserts the token for key, preserving all other entries.
func (s *FileStore) Save(key string, token *StoredToken) error {
	fc, err := s.load()
	if err != nil {
		return err
	}
	fc.Tokens[key] = token
	return s.save(fc)
}

// Load returns the token for key, or ErrTokenNotFound.
func (s *FileStore) Load(key string) (*StoredToken, error) {
	fc, err := s.load()
	if err != nil {
		return nil, err
	}
	t, ok := fc.Tokens[key]
	if !ok {
		return nil, ErrTokenNotFound
	}
	return t, nil
}

// Delete removes the entry for key. A missing entry is not an error.
func (s *FileStore) Delete(key string) error {
	fc, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := fc.Tokens[key]; !ok {
		return nil
	}
	delete(fc.Tokens, key)
	return s.save(fc)
}

// Backend reports "file".
func (s *FileStore) Backend() string { return backendFile }
