package transfer

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrepareFileAndDirectoryZIP(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "报告 file.txt")
	content := []byte("hello ichuansu")
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSource(filePath)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Cleanup()
	if prepared.Name != "报告 file.txt" || prepared.Size != int64(len(content)) {
		t.Fatalf("unexpected prepared file: %#v", prepared)
	}
	want := sha256.Sum256(content)
	if prepared.SHA256 != hex.EncodeToString(want[:]) {
		t.Fatalf("hash mismatch: %s", prepared.SHA256)
	}

	root := filepath.Join(dir, "project")
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("A"), 0o600); err != nil {
		t.Fatal(err)
	}
	preparedDir, err := PrepareSource(root)
	if err != nil {
		t.Fatal(err)
	}
	defer preparedDir.Cleanup()
	if preparedDir.Name != "project.zip" {
		t.Fatalf("ZIP name = %q", preparedDir.Name)
	}
	archive, err := zip.OpenReader(preparedDir.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	seen := map[string]bool{}
	for _, entry := range archive.File {
		if strings.HasPrefix(entry.Name, "/") || strings.Contains(entry.Name, "..") || strings.Contains(entry.Name, "\\") {
			t.Fatalf("unsafe ZIP entry: %q", entry.Name)
		}
		seen[entry.Name] = true
	}
	if !seen["project/"] || !seen["project/a.txt"] || !seen["project/empty/"] {
		t.Fatalf("missing top-level or empty directory entries: %#v", seen)
	}
}

func TestZipSkipsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}
	dir := t.TempDir()
	root := filepath.Join(dir, "tree")
	outside := filepath.Join(dir, "outside.txt")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	prepared, err := PrepareSource(root)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Cleanup()
	archive, err := zip.OpenReader(prepared.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) != 1 || archive.File[0].Name != "tree/" {
		t.Fatalf("symlink should be omitted: %#v", archive.File)
	}
}

func TestSafeFilename(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "../escape", `a\\b`, `C:\\file`, "x\ny", "CON", "Lpt1.txt", " trailing."} {
		if _, err := SafeFilename(bad); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("SafeFilename(%q) error=%v", bad, err)
		}
	}
	for _, good := range []string{"中文 文件.zip", "report.pdf", "a-b_c.txt"} {
		if got, err := SafeFilename(good); err != nil || got != good {
			t.Errorf("SafeFilename(%q)=(%q,%v)", good, got, err)
		}
	}
}

func TestSaveAtomicVerifiesHashAndNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("binary content\x00\x01")
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	path, err := SaveAtomic(dir, "report.bin", bytes.NewReader(payload), int64(len(payload)), hash, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("saved content mismatch: %q", got)
	}
	if _, err := SaveAtomic(dir, "report.bin", bytes.NewReader(payload), int64(len(payload)), hash, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, err := SaveAtomic(dir, "bad.bin", bytes.NewReader(payload), int64(len(payload)), strings.Repeat("0", 64), nil); !errors.Is(err, ErrChecksum) {
		t.Fatalf("expected checksum error, got %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".part") {
			t.Fatalf("temporary partial file remains: %s", entry.Name())
		}
	}
}

func TestSaveAtomicRejectsTruncatedAndExtendedStreams(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("payload")
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	if _, err := SaveAtomic(dir, "short.bin", bytes.NewReader(payload[:2]), int64(len(payload)), hash, nil); err == nil {
		t.Fatal("expected truncated stream error")
	}
	if _, err := SaveAtomic(dir, "long.bin", bytes.NewReader(append(payload, 0)), int64(len(payload)), hash, nil); err == nil {
		t.Fatal("expected extended stream error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial files were not cleaned: %v", entries)
	}
}

func TestSaveAtomicRejectsTraversalBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	if _, err := SaveAtomic(dir, "../escape", bytes.NewReader([]byte("x")), 1, strings.Repeat("0", 64), nil); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("got %v", err)
	}
	if _, err := io.Copy(io.Discard, bytes.NewReader(([]byte("still fine")))); err != nil {
		t.Fatal(err)
	}
}

