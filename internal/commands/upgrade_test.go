package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetectPlatform(t *testing.T) {
	osName, archName, err := detectPlatform()
	if err != nil {
		t.Fatalf("detectPlatform() error: %v", err)
	}

	if osName != runtime.GOOS {
		t.Errorf("osName = %q, want %q", osName, runtime.GOOS)
	}
	if archName != runtime.GOARCH {
		t.Errorf("archName = %q, want %q", archName, runtime.GOARCH)
	}
}

func TestVerifyChecksum_Valid(t *testing.T) {
	dir := t.TempDir()

	content := []byte("hello wherobots")
	assetPath := filepath.Join(dir, "wherobots_darwin_arm64")
	if err := os.WriteFile(assetPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256(content)
	checksum := hex.EncodeToString(h[:])

	checksumFile := filepath.Join(dir, "checksums.txt")
	checksumContent := checksum + "  wherobots_darwin_arm64\n" +
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  wherobots_linux_amd64\n"
	if err := os.WriteFile(checksumFile, []byte(checksumContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := verifyChecksum(assetPath, checksumFile, "wherobots_darwin_arm64"); err != nil {
		t.Fatalf("verifyChecksum should pass with correct hash, got: %v", err)
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	dir := t.TempDir()

	assetPath := filepath.Join(dir, "wherobots_darwin_arm64")
	if err := os.WriteFile(assetPath, []byte("real content"), 0644); err != nil {
		t.Fatal(err)
	}

	checksumFile := filepath.Join(dir, "checksums.txt")
	checksumContent := "0000000000000000000000000000000000000000000000000000000000000000  wherobots_darwin_arm64\n"
	if err := os.WriteFile(checksumFile, []byte(checksumContent), 0644); err != nil {
		t.Fatal(err)
	}

	err := verifyChecksum(assetPath, checksumFile, "wherobots_darwin_arm64")
	if err == nil {
		t.Fatal("verifyChecksum should fail on mismatch")
	}
}

func TestVerifyChecksum_MissingEntry(t *testing.T) {
	dir := t.TempDir()

	assetPath := filepath.Join(dir, "wherobots_darwin_arm64")
	if err := os.WriteFile(assetPath, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	checksumFile := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(checksumFile, []byte("abc123  wherobots_linux_amd64\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := verifyChecksum(assetPath, checksumFile, "wherobots_darwin_arm64")
	if err == nil {
		t.Fatal("verifyChecksum should fail when asset not in checksums file")
	}
}

func TestInstallBinary(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "src-binary")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi"), 0644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "wherobots")
	if err := installBinary(src, dst); err != nil {
		t.Fatalf("installBinary() error: %v", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("installed binary not found: %v", err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Error("installed binary should be executable")
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "#!/bin/sh\necho hi" {
		t.Errorf("binary content mismatch: %s", got)
	}
}
