package tools

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeTencentArchive(t *testing.T, path string, binary []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: path, Mode: 0o755, Size: int64(len(binary))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}

func TestEnsureTencentCLIVerifiesOfficialPackageAndInstallsOnlyExpectedBinary(t *testing.T) {
	binary := []byte("fake Tencent CLI binary")
	archive := makeTencentArchive(t, "package/bin/tencent-channel-cli", binary)
	digest := sha512.Sum512(archive)
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(archive)), Header: make(http.Header)}, nil
	})}
	packageInfo := tencentPackage{
		Name:       "test",
		Tarball:    "https://registry.npmjs.org/test/-/test.tgz",
		Integrity:  "sha512-" + base64.StdEncoding.EncodeToString(digest[:]),
		BinaryPath: "package/bin/tencent-channel-cli",
	}
	root := t.TempDir()
	path, err := ensureTencentCLIWithPackage(context.Background(), root, packageInfo, "darwin", client)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(binary) {
		t.Fatalf("installed binary = %q, err = %v", got, err)
	}
	if filepath.Base(path) != "tencent-channel-cli" || !strings.Contains(path, filepath.Join("tools", "tencent-channel-cli", tencentCLIVersion)) {
		t.Fatalf("unexpected install path: %s", path)
	}
}

func TestEnsureTencentCLIWindowsUsesTheWindowsBinaryName(t *testing.T) {
	binary := []byte("fake Tencent Windows CLI binary")
	archive := makeTencentArchive(t, "package/bin/tencent-channel-cli.exe", binary)
	digest := sha512.Sum512(archive)
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(archive)), Header: make(http.Header)}, nil
	})}
	path, err := ensureTencentCLIWithPackage(context.Background(), t.TempDir(), tencentPackage{
		Tarball:    "https://registry.npmjs.org/test/-/test.tgz",
		Integrity:  "sha512-" + base64.StdEncoding.EncodeToString(digest[:]),
		BinaryPath: "package/bin/tencent-channel-cli.exe",
	}, "windows", client)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "tencent-channel-cli.exe" {
		t.Fatalf("unexpected Windows install path: %q", path)
	}
}

func TestEnsureTencentCLIRejectsNonOfficialPackageURL(t *testing.T) {
	root := t.TempDir()
	_, err := ensureTencentCLIWithPackage(context.Background(), root, tencentPackage{
		Tarball:    "https://registry.npmjs.org.evil.example/test.tgz",
		Integrity:  "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, sha512.Size)),
		BinaryPath: "package/bin/tencent-channel-cli",
	}, "darwin", nil)
	if err == nil || !strings.Contains(err.Error(), "official HTTPS registry URL") {
		t.Fatalf("unexpected non-official URL error: %v", err)
	}
}

func TestEnsureTencentCLIRejectsUnsafeRedirect(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"http://insecure.example.test/package.tgz"}},
			Request:    request,
			Body:       http.NoBody,
		}, nil
	})}
	root := t.TempDir()
	_, err := ensureTencentCLIWithPackage(context.Background(), root, tencentPackage{
		Tarball:    "https://registry.npmjs.org/test/-/test.tgz",
		Integrity:  "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, sha512.Size)),
		BinaryPath: "package/bin/tencent-channel-cli",
	}, "darwin", client)
	if err == nil || !strings.Contains(err.Error(), "reject Tencent Channel CLI redirect") {
		t.Fatalf("unexpected redirect error: %v", err)
	}
}

func TestEnsureTencentCLIRejectsInvalidBinaryPath(t *testing.T) {
	root := t.TempDir()
	_, err := ensureTencentCLIWithPackage(context.Background(), root, tencentPackage{
		Tarball:    "https://registry.npmjs.org/test/-/test.tgz",
		Integrity:  "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, sha512.Size)),
		BinaryPath: "../outside",
	}, "darwin", nil)
	if err == nil || !strings.Contains(err.Error(), "binary path is invalid") {
		t.Fatalf("unexpected binary path error: %v", err)
	}
}

func TestEnsureTencentCLIRejectsIntegrityMismatchAndDoesNotLeaveFile(t *testing.T) {
	archive := makeTencentArchive(t, "package/bin/tencent-channel-cli", []byte("tampered"))
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(archive)), Header: make(http.Header)}, nil
	})}
	root := t.TempDir()
	_, err := ensureTencentCLIWithPackage(context.Background(), root, tencentPackage{
		Tarball:    "https://registry.npmjs.org/test/-/test.tgz",
		Integrity:  "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, sha512.Size)),
		BinaryPath: "package/bin/tencent-channel-cli",
	}, "darwin", client)
	if err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("unexpected integrity error: %v", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(root, "tools"))
	if readErr == nil && len(entries) != 0 {
		t.Fatalf("unexpected files after integrity failure: %#v", entries)
	}
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestExtractTencentBinaryRejectsMissingExpectedEntry(t *testing.T) {
	archive := makeTencentArchive(t, "package/other", []byte("not cli"))
	_, err := extractTencentBinary(archive, "package/bin/tencent-channel-cli")
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("unexpected missing entry error: %v", err)
	}
	if _, err := io.ReadAll(bytes.NewReader(archive)); err != nil {
		t.Fatal(err)
	}
}

func TestValidInstalledTencentCLIVerifiesMarkerAndBinary(t *testing.T) {
	root := t.TempDir()
	binaryPath := filepath.Join(root, "tencent-channel-cli")
	markerPath := filepath.Join(root, "install.json")
	binary := []byte("verified")
	if err := os.WriteFile(binaryPath, binary, 0o700); err != nil {
		t.Fatal(err)
	}
	binaryDigest := sha256.Sum256(binary)
	marker, err := json.Marshal(tencentInstallMarker{PackageIntegrity: "sha512-pinned", BinarySHA256: hex.EncodeToString(binaryDigest[:])})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, marker, 0o600); err != nil {
		t.Fatal(err)
	}
	valid, err := validInstalledTencentCLI(binaryPath, markerPath, "sha512-pinned")
	if err != nil || !valid {
		t.Fatalf("valid installation rejected: valid=%v err=%v", valid, err)
	}
	if err := os.WriteFile(binaryPath, []byte("replaced"), 0o700); err != nil {
		t.Fatal(err)
	}
	valid, err = validInstalledTencentCLI(binaryPath, markerPath, "sha512-pinned")
	if err != nil || valid {
		t.Fatalf("replaced binary accepted: valid=%v err=%v", valid, err)
	}
}
