package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	keyring "github.com/zalando/go-keyring"
)

// ResolveOptions controls backend selection.
type ResolveOptions struct {
	// ForceFile skips the keyring probe and uses the encrypted file backend.
	ForceFile bool
	// Password is the master password for the file backend. Required when the
	// keyring is unavailable or ForceFile is set.
	Password string
	// FilePath overrides the default token file location (mainly for tests).
	FilePath string
}

// Resolve picks a backend: the OS keyring when available and writable,
// otherwise the encrypted file backend (which requires a master password).
func Resolve(opts ResolveOptions) (Store, error) {
	if !opts.ForceFile && probeKeyring() == nil {
		return &KeyringStore{}, nil
	}
	if opts.Password == "" {
		return nil, errors.New("storage: the encrypted file backend (used when " +
			"the OS keyring is unavailable or ForceFile is set) requires a master " +
			"password; set JIRA_MASTER_PASSWORD")
	}
	path := opts.FilePath
	if path == "" {
		var err error
		if path, err = defaultFilePath(); err != nil {
			return nil, err
		}
	}
	return &FileStore{Path: path, Password: []byte(opts.Password)}, nil
}

// probeKeyring verifies the keyring can be written and deleted. The probe key
// is unique per process so concurrent probes don't delete each other's entry,
// and a not-found on Delete is treated as success (the entry may have been
// removed by a racing probe) — only a genuine set/delete failure means the
// keyring is unavailable and Resolve should fall back to the file backend.
func probeKeyring() error {
	probeKey := fmt.Sprintf("__probe__%d", os.Getpid())
	if err := keyring.Set(keyringService, probeKey, "1"); err != nil {
		return err
	}
	if err := keyring.Delete(keyringService, probeKey); err != nil &&
		!errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// defaultFilePath returns ~/.config/go-jira/tokens.enc (or the OS equivalent).
func defaultFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("storage: locate config dir: %w", err)
	}
	return filepath.Join(dir, "go-jira", "tokens.enc"), nil
}
