package server

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateTokenPersistsRandomKey(t *testing.T) {
	dir := t.TempDir()
	first, created, err := LoadOrCreateToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !created || len(first) != 32 {
		t.Fatalf("created=%v token length=%d", created, len(first))
	}
	second, created, err := LoadOrCreateToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	if created || !bytes.Equal(first, second) {
		t.Fatal("token was not reused")
	}
	loaded, err := LoadToken(dir)
	if err != nil || !bytes.Equal(first, loaded) {
		t.Fatalf("LoadToken mismatch: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, tokenFilename))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 32 {
		t.Fatal("token file size = %d", info.Size())
	}
}

func TestLoadTokenRejectsSymlinkAndInvalidLength(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, tokenFilename), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(dir); err == nil {
		t.Fatal("expected invalid token length")
	}
}
