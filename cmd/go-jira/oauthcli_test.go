package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestAtomicWriteFile_CreatesParentDir verifies the rotation writer creates a
// missing parent directory and writes the content with the requested mode, so
// a rotated refresh token is never silently lost when the output path's
// directory does not yet exist.
func TestAtomicWriteFile_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deeper", "refresh.token")

	if err := atomicWriteFile(path, []byte("rotated-token")); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "rotated-token" {
		t.Errorf("content = %q, want %q", got, "rotated-token")
	}

	// Permission bits are only meaningful on Unix; Windows models just the
	// read-only bit, so Perm() would not equal 0o600 there even on a correct
	// write. Keep the content/atomicity assertions cross-platform.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("perm = %o, want 600", perm)
		}
	}

	// No stray temp files left behind in the target directory.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory has %d entries, want 1 (no leftover temp file)", len(entries))
	}
}

// TestAtomicWriteFile_Overwrites verifies a second write atomically replaces
// the previous contents (the rename-over-existing case).
func TestAtomicWriteFile_Overwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "refresh.token")

	if err := atomicWriteFile(path, []byte("first")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := atomicWriteFile(path, []byte("second")); err != nil {
		t.Fatalf("second write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("content = %q, want %q", got, "second")
	}
}
