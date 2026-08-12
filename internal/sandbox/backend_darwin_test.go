//go:build darwin

package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinBackendReportsMissingVFKitWithoutHostFallback(t *testing.T) {
	root := t.TempDir()
	executor := newPlatformExecutor(root, "darwin", "arm64", ImageSource{}, Detect("darwin", "arm64"))
	status := executor.Status(context.Background())
	if !status.Supported || status.Ready || !strings.Contains(status.Reason, "vfkit") {
		t.Fatalf("unexpected missing vfkit status: %#v", status)
	}
	if _, err := executor.Execute(context.Background(), Request{Command: "echo"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing vfkit must not execute on host: %v", err)
	}
}

func TestDarwinBackendRequiresGuestRuntimePingBeforeReady(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sandbox"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sandbox", "image.json"), []byte(`{"platform":"darwin/arm64","disk":"images/runtime.disk","sha256":"`+strings.Repeat("0", 64)+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &vfkitExecutor{
		dataRoot:      root,
		vfkit:         filepath.Join(root, "tools", "vfkit", vfkitVersion, vfkitExecutable),
		vfkitSource:   "station",
		runtimeSocket: filepath.Join(root, "sandbox", vfkitRuntimeSocket),
		process:       &exec.Cmd{},
		status:        Detect("darwin", "arm64"),
	}

	status := executor.Status(context.Background())
	if status.Ready || !strings.Contains(status.Reason, "runtime is not ready") {
		t.Fatalf("runtime without a successful ping was reported ready: %#v", status)
	}
	if _, err := executor.Execute(context.Background(), Request{Command: "echo"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("runtime without a successful ping was executable: %v", err)
	}

	executor.runtimeReady = true
	status = executor.Status(context.Background())
	if !status.Ready || !strings.Contains(status.Reason, "ready") {
		t.Fatalf("runtime after a successful ping was not reported ready: %#v", status)
	}
}

func TestDarwinPurgeClearsRemovedVFKitState(t *testing.T) {
	root := t.TempDir()
	executor := &vfkitExecutor{
		dataRoot:      root,
		vfkit:         filepath.Join(root, "tools", "vfkit", vfkitVersion, vfkitExecutable),
		vfkitSource:   "station",
		runtimeSocket: filepath.Join(root, "sandbox", vfkitRuntimeSocket),
		status:        Detect("darwin", "arm64"),
	}
	if err := executor.Purge(context.Background()); err != nil {
		t.Fatal(err)
	}
	if executor.vfkit != "" || executor.vfkitSource != "" {
		t.Fatalf("purge retained removed vfkit state: path=%q source=%q", executor.vfkit, executor.vfkitSource)
	}
	status := executor.Status(context.Background())
	if status.Ready || !strings.Contains(status.Reason, "vfkit sidecar") {
		t.Fatalf("unexpected post-purge status: %#v", status)
	}
}

func TestEnsureVFKitVerifiesAndInstallsPinnedBinary(t *testing.T) {
	binary := []byte("verified vfkit binary")
	digest := sha256.Sum256(binary)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(binary)
	}))
	defer server.Close()
	root := t.TempDir()
	packageInfo := vfkitPackage{URL: "https://github.com/crc-org/vfkit/releases/download/v0.6.4/vfkit", SHA256: hex.EncodeToString(digest[:])}
	client := server.Client()
	packageInfo.URL = "https://github.com/crc-org/vfkit/releases/download/v0.6.4/vfkit"
	client.Transport = rewriteVFKitTestTransport{base: server.Client().Transport, target: packageInfo.URL, replacement: server.URL}
	path, err := ensureVFKitWithPackage(context.Background(), root, packageInfo, client)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != vfkitExecutable || !isExecutableFile(path) {
		t.Fatalf("installed vfkit path = %q", path)
	}
	if _, err := os.Stat(filepath.Join(root, "tools", "vfkit", vfkitVersion, "install.json")); err != nil {
		t.Fatalf("vfkit marker missing: %v", err)
	}
	if valid, err := validVFKitInstall(path, filepath.Join(filepath.Dir(path), "install.json"), packageInfo); err != nil || !valid {
		t.Fatalf("installed vfkit is not reusable: valid=%v err=%v", valid, err)
	}
}

func TestResolveVFKitPathRejectsCorruptMarker(t *testing.T) {
	binary := []byte("verified vfkit binary")
	digest := sha256.Sum256(binary)
	installRoot := filepath.Join(t.TempDir(), "tools", "vfkit", vfkitVersion)
	binaryPath := filepath.Join(installRoot, vfkitExecutable)
	if err := os.MkdirAll(installRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binaryPath, binary, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "install.json"), []byte(`{"version":"v0.6.4","sha256":"`+hex.EncodeToString(digest[:])+`"`), 0o600); err != nil {
		t.Fatal(err)
	}
	if path, source := resolveVFKitPath(filepath.Dir(filepath.Dir(filepath.Dir(installRoot)))); path != "" || source != "" {
		t.Fatalf("corrupt marker was accepted: path=%q source=%q", path, source)
	}
}

func TestEnsureVFKitRejectsNonOfficialPackageURL(t *testing.T) {
	root := t.TempDir()
	_, err := ensureVFKitWithPackage(context.Background(), root, vfkitPackage{
		URL:    "https://downloads.example.test/vfkit",
		SHA256: pinnedVFKitPackage.SHA256,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "fixed official GitHub HTTPS URL") {
		t.Fatalf("unexpected non-official URL error: %v", err)
	}
}

func TestEnsureVFKitDigestMismatchLeavesNoInstalledBinary(t *testing.T) {
	binary := []byte("tampered vfkit binary")
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(binary)
	}))
	defer server.Close()
	root := t.TempDir()
	packageInfo := vfkitPackage{URL: pinnedVFKitPackage.URL, SHA256: strings.Repeat("0", sha256.Size*2)}
	client := server.Client()
	client.Transport = rewriteVFKitTestTransport{base: server.Client().Transport, target: packageInfo.URL, replacement: server.URL}
	if _, err := ensureVFKitWithPackage(context.Background(), root, packageInfo, client); err == nil || !strings.Contains(err.Error(), "digest does not match") {
		t.Fatalf("unexpected digest mismatch error: %v", err)
	}
	installRoot := filepath.Join(root, "tools", "vfkit", vfkitVersion)
	if _, err := os.Stat(filepath.Join(installRoot, vfkitExecutable)); !os.IsNotExist(err) {
		t.Fatalf("digest mismatch left installed binary: %v", err)
	}
	if _, err := os.Stat(filepath.Join(installRoot, "install.json")); !os.IsNotExist(err) {
		t.Fatalf("digest mismatch left install marker: %v", err)
	}
	entries, err := os.ReadDir(installRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("digest mismatch left temporary files: %v", entries)
	}
}

func TestEnsureVFKitRejectsUnsafeRedirect(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"http://insecure.example.test/vfkit"}},
				Request:    request,
				Body:       http.NoBody,
			}, nil
		}),
	}
	root := t.TempDir()
	_, err := ensureVFKitWithPackage(context.Background(), root, vfkitPackage{
		URL:    pinnedVFKitPackage.URL,
		SHA256: pinnedVFKitPackage.SHA256,
	}, client)
	if err == nil || !strings.Contains(err.Error(), "reject vfkit redirect") {
		t.Fatalf("unexpected redirect error: %v", err)
	}
}

func TestVFKitRedirectPolicyAllowsOnlyGitHubSignedReleaseAssets(t *testing.T) {
	allowed, err := http.NewRequest(http.MethodGet, "https://release-assets.githubusercontent.com/github-production-release-asset/example?sig=temporary", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := vfkitRedirectPolicy(allowed, nil); err != nil {
		t.Fatalf("signed GitHub release asset was rejected: %v", err)
	}
	for _, value := range []string{
		"https://release-assets.githubusercontent.com/github-production-release-asset/example",
		"https://example.test/vfkit?sig=temporary",
		"http://release-assets.githubusercontent.com/vfkit?sig=temporary",
	} {
		request, requestErr := http.NewRequest(http.MethodGet, value, nil)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		if err := vfkitRedirectPolicy(request, nil); err == nil {
			t.Fatalf("unsafe vfkit redirect was accepted: %s", value)
		}
	}
}

type rewriteVFKitTestTransport struct {
	base        http.RoundTripper
	target      string
	replacement string
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func (transport rewriteVFKitTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	if clone.URL.String() == transport.target {
		clone.URL.Scheme = "https"
		clone.URL.Host = strings.TrimPrefix(transport.replacement, "https://")
		clone.URL.Path = "/"
	}
	return transport.base.RoundTrip(clone)
}
