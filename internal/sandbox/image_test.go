package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifySandboxArtifactRestrictsPathAndDigest(t *testing.T) {
	root := t.TempDir()
	artifactPath := filepath.Join(root, "sandbox", "images", "runtime.raw")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o700); err != nil {
		t.Fatal(err)
	}
	contents := []byte("guest image")
	if err := os.WriteFile(artifactPath, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	path, err := verifySandboxArtifact(root, "images/runtime.raw", expectedSandboxPlatform(), hex.EncodeToString(digest[:]))
	if err != nil || path != artifactPath {
		t.Fatalf("verify artifact = %q, %v", path, err)
	}
	if _, err := verifySandboxArtifact(root, "../outside", expectedSandboxPlatform(), hex.EncodeToString(digest[:])); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("path traversal was accepted: %v", err)
	}
	if _, err := verifySandboxArtifact(root, "images/runtime.raw", expectedSandboxPlatform(), strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("bad digest was accepted: %v", err)
	}
}

func TestVerifySandboxArtifactRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.raw")
	contents := []byte("outside guest image")
	if err := os.WriteFile(outside, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	artifactPath := filepath.Join(root, "sandbox", "images", "runtime.raw")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, artifactPath); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	digest := sha256.Sum256(contents)
	if _, err := verifySandboxArtifact(root, "images/runtime.raw", expectedSandboxPlatform(), hex.EncodeToString(digest[:])); err == nil || !strings.Contains(err.Error(), "symbolic") {
		t.Fatalf("symlink artifact was accepted: %v", err)
	}
}

func TestVerifySandboxArtifactRejectsSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sandbox"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "sandbox", "images")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	contents := []byte("outside guest image")
	artifactPath := filepath.Join(outside, "runtime.raw")
	if err := os.WriteFile(artifactPath, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	if _, err := verifySandboxArtifact(root, "images/runtime.raw", expectedSandboxPlatform(), hex.EncodeToString(digest[:])); err == nil || !strings.Contains(err.Error(), "symbolic") {
		t.Fatalf("symlinked parent was accepted: %v", err)
	}
}
