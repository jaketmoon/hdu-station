package sandbox

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureSandboxImageDownloadsAndInstallsDigestNamedArtifact(t *testing.T) {
	contents := []byte("verified sandbox image")
	digest := sha256.Sum256(contents)
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(privateKey, digest[:])
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(contents)
	}))
	defer server.Close()

	root := t.TempDir()
	manifest, err := ensureSandboxImageWithClient(context.Background(), root, ImageSource{
		URL:       server.URL,
		SHA256:    hex.EncodeToString(digest[:]),
		Signature: base64.StdEncoding.EncodeToString(signature),
		PublicKey: base64.StdEncoding.EncodeToString(publicKey),
	}, expectedSandboxPlatform(), "disk", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(root, "sandbox", "images", "runtime.disk-"+hex.EncodeToString(digest[:]))
	if manifest.Disk != filepath.ToSlash(filepath.Join("images", "runtime.disk-"+hex.EncodeToString(digest[:]))) {
		t.Fatalf("manifest disk = %q", manifest.Disk)
	}
	if got, err := os.ReadFile(wantPath); err != nil || string(got) != string(contents) {
		t.Fatalf("downloaded artifact = %q, err = %v", got, err)
	}
	data, err := os.ReadFile(filepath.Join(root, "sandbox", "image.json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted sandboxImageManifest
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != manifest {
		t.Fatalf("persisted manifest = %#v, want %#v", persisted, manifest)
	}

	if _, err := ensureSandboxImageWithClient(context.Background(), root, ImageSource{
		URL:       server.URL,
		SHA256:    strings.ToUpper(hex.EncodeToString(digest[:])),
		Signature: base64.StdEncoding.EncodeToString(signature),
		PublicKey: base64.StdEncoding.EncodeToString(publicKey),
	}, expectedSandboxPlatform(), "disk", server.Client()); err != nil {
		t.Fatalf("reusing verified digest-named artifact: %v", err)
	}
}

func TestEnsureSandboxImageRejectsInvalidSignature(t *testing.T) {
	contents := []byte("signed payload")
	digest := sha256.Sum256(contents)
	publicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(contents)
	}))
	defer server.Close()

	root := t.TempDir()
	_, err = ensureSandboxImageWithClient(context.Background(), root, ImageSource{
		URL:       server.URL,
		SHA256:    hex.EncodeToString(digest[:]),
		Signature: base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)),
		PublicKey: base64.StdEncoding.EncodeToString(publicKey),
	}, expectedSandboxPlatform(), "disk", server.Client())
	if err == nil || !strings.Contains(err.Error(), "signature does not match") {
		t.Fatalf("unexpected signature error: %v", err)
	}
}

func TestEnsureSandboxImageRejectsDigestMismatchWithoutManifest(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("tampered sandbox image"))
	}))
	defer server.Close()

	root := t.TempDir()
	_, err := ensureSandboxImageWithClient(context.Background(), root, ImageSource{
		URL:    server.URL,
		SHA256: strings.Repeat("c", 64),
	}, expectedSandboxPlatform(), "archive", server.Client())
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("unexpected digest mismatch error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "sandbox", "image.json")); !os.IsNotExist(statErr) {
		t.Fatalf("manifest should not be installed after digest mismatch: %v", statErr)
	}
}

func TestEnsureSandboxImageRejectsInsecureRedirect(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("must not be downloaded"))
	}))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, plain.URL, http.StatusFound)
	}))
	defer secure.Close()

	root := t.TempDir()
	_, err := ensureSandboxImageWithClient(context.Background(), root, ImageSource{
		URL:    secure.URL,
		SHA256: strings.Repeat("d", 64),
	}, expectedSandboxPlatform(), "disk", secure.Client())
	if err == nil || !strings.Contains(err.Error(), "reject sandbox image redirect") {
		t.Fatalf("unexpected redirect error: %v", err)
	}
}

func TestEnsureSandboxImageRejectsSymlinkedDestinationBeforeDownload(t *testing.T) {
	contents := []byte("verified sandbox image")
	digest := sha256.Sum256(contents)
	serverCalled := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		serverCalled = true
		_, _ = writer.Write(contents)
	}))
	defer server.Close()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sandbox"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "sandbox", "images")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	_, err := ensureSandboxImageWithClient(context.Background(), root, ImageSource{
		URL:    server.URL,
		SHA256: hex.EncodeToString(digest[:]),
	}, expectedSandboxPlatform(), "disk", server.Client())
	if err == nil || !strings.Contains(err.Error(), "symbolic") {
		t.Fatalf("symlinked download destination was accepted: %v", err)
	}
	if serverCalled {
		t.Fatal("image download started through a symlinked destination")
	}
	if entries, readErr := os.ReadDir(outside); readErr != nil || len(entries) != 0 {
		t.Fatalf("outside image directory was modified: entries=%v err=%v", entries, readErr)
	}
}
