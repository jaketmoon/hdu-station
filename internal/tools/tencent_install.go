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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	tencentCLIVersion = "1.0.10"
	maxTencentPackage = 32 << 20
)

type tencentPackage struct {
	Name       string
	Tarball    string
	Integrity  string
	BinaryPath string
}

type tencentInstallMarker struct {
	PackageIntegrity string `json:"package_integrity"`
	BinarySHA256     string `json:"binary_sha256"`
}

func (packageInfo tencentPackage) validate() error {
	parsed, err := url.Parse(packageInfo.Tarball)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "registry.npmjs.org" || parsed.Path == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return errors.New("Tencent Channel CLI package URL is not an official HTTPS registry URL")
	}
	prefix, encoded, ok := strings.Cut(strings.TrimSpace(packageInfo.Integrity), "-")
	decoded, decodeErr := base64.StdEncoding.DecodeString(encoded)
	if !ok || prefix != "sha512" || decodeErr != nil || len(decoded) != sha512.Size {
		return errors.New("Tencent Channel CLI package integrity is not SHA-512")
	}
	normalizedBinaryPath := path.Clean(packageInfo.BinaryPath)
	if strings.TrimSpace(packageInfo.BinaryPath) == "" || path.IsAbs(normalizedBinaryPath) || normalizedBinaryPath != packageInfo.BinaryPath || strings.HasPrefix(normalizedBinaryPath, "../") {
		return errors.New("Tencent Channel CLI package binary path is invalid")
	}
	return nil
}

// The entries are copied from the official npm registry metadata for the
// pinned Tencent Channel CLI release. No user-provided URL is accepted.
var tencentPackages = map[string]tencentPackage{
	"darwin/arm64": {
		Name:       "tencent-channel-cli-darwin-arm64",
		Tarball:    "https://registry.npmjs.org/tencent-channel-cli-darwin-arm64/-/tencent-channel-cli-darwin-arm64-1.0.10.tgz",
		Integrity:  "sha512-XNIBdSHPtEPd2L6yEf1TcVMUBLf5yt+phMvmxNKSd6DWpOrXJanSKGlg1YfRCnZEaZticlEFQeDpmQedkHBeTQ==",
		BinaryPath: "package/bin/tencent-channel-cli",
	},
	"darwin/amd64": {
		Name:       "tencent-channel-cli-darwin-x64",
		Tarball:    "https://registry.npmjs.org/tencent-channel-cli-darwin-x64/-/tencent-channel-cli-darwin-x64-1.0.10.tgz",
		Integrity:  "sha512-7zmgcuqrec0nutg+24bXFKrcG6ZFxkAGtdflhj2uNHqbWQhVC+fHqA+Y5+cZBmzG86XO4jlPFpwO9TzU3hzHvw==",
		BinaryPath: "package/bin/tencent-channel-cli",
	},
	"linux/arm64": {
		Name:       "tencent-channel-cli-linux-arm64",
		Tarball:    "https://registry.npmjs.org/tencent-channel-cli-linux-arm64/-/tencent-channel-cli-linux-arm64-1.0.10.tgz",
		Integrity:  "sha512-joSAzJYG4cb8HP9or7luXLjEOX4xFfrbfIK1MInbP/D3Oxvanp6SrYxXvsEOG/TdlVeFSoyYW4+hiL6xmdsEYg==",
		BinaryPath: "package/bin/tencent-channel-cli",
	},
	"linux/amd64": {
		Name:       "tencent-channel-cli-linux-x64",
		Tarball:    "https://registry.npmjs.org/tencent-channel-cli-linux-x64/-/tencent-channel-cli-linux-x64-1.0.10.tgz",
		Integrity:  "sha512-DLjAz51H8iV1zRLUXv+u4Z2VaZDegsDLk/4iJ93Qjpsa/oEgDn0KHh94Cwvq7vRJA60h6IBy2O7s/GpV4jUXVg==",
		BinaryPath: "package/bin/tencent-channel-cli",
	},
	"windows/amd64": {
		Name:       "tencent-channel-cli-win32-x64",
		Tarball:    "https://registry.npmjs.org/tencent-channel-cli-win32-x64/-/tencent-channel-cli-win32-x64-1.0.10.tgz",
		Integrity:  "sha512-9IOpHxB+KFaKjauR8TAQkEu3yuMoTmTT+T8MchHl9Vv95R76RER7X8TZliup5La1e499HqpdRIHB/lwS+ku1eg==",
		BinaryPath: "package/bin/tencent-channel-cli.exe",
	},
}

func tencentPackageFor(goos, goarch string) (tencentPackage, error) {
	packageInfo, ok := tencentPackages[goos+"/"+goarch]
	if !ok {
		return tencentPackage{}, fmt.Errorf("Tencent Channel CLI does not support %s/%s", goos, goarch)
	}
	return packageInfo, nil
}

func ensureTencentCLI(ctx context.Context, dataRoot, goos, goarch string, client *http.Client) (string, error) {
	if strings.TrimSpace(dataRoot) == "" {
		return "", errors.New("Tencent Channel data root is required")
	}
	packageInfo, err := tencentPackageFor(goos, goarch)
	if err != nil {
		return "", err
	}
	return ensureTencentCLIWithPackage(ctx, dataRoot, packageInfo, goos, client)
}

func EnsureTencentCLI(ctx context.Context, dataRoot, goos, goarch string, client *http.Client) (string, error) {
	return ensureTencentCLI(ctx, dataRoot, goos, goarch, client)
}

func ensureTencentCLIWithPackage(ctx context.Context, dataRoot string, packageInfo tencentPackage, goos string, client *http.Client) (string, error) {
	if err := packageInfo.validate(); err != nil {
		return "", err
	}
	installRoot := filepath.Join(dataRoot, "tools", "tencent-channel-cli", tencentCLIVersion)
	binaryName := "tencent-channel-cli"
	if goos == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(installRoot, binaryName)
	markerPath := filepath.Join(installRoot, "install.json")
	if valid, err := validInstalledTencentCLI(binaryPath, markerPath, packageInfo.Integrity); err == nil && valid {
		return binaryPath, nil
	}
	if client == nil {
		client = tencentHTTPClient()
	} else {
		copy := *client
		copy.CheckRedirect = tencentRedirectPolicy
		if copy.Timeout <= 0 {
			copy.Timeout = 10 * time.Minute
		}
		client = &copy
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, packageInfo.Tarball, nil)
	if err != nil {
		return "", fmt.Errorf("create Tencent Channel CLI download request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("download Tencent Channel CLI: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("download Tencent Channel CLI: server returned HTTP %d", response.StatusCode)
	}
	archive, err := io.ReadAll(io.LimitReader(response.Body, maxTencentPackage+1))
	if err != nil {
		return "", fmt.Errorf("read Tencent Channel CLI package: %w", err)
	}
	if len(archive) > maxTencentPackage {
		return "", errors.New("Tencent Channel CLI package exceeds the 32 MiB limit")
	}
	if err := verifyTencentIntegrity(archive, packageInfo.Integrity); err != nil {
		return "", err
	}
	binary, err := extractTencentBinary(archive, packageInfo.BinaryPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(installRoot, 0o700); err != nil {
		return "", fmt.Errorf("create Tencent Channel CLI directory: %w", err)
	}
	temporary, err := os.CreateTemp(installRoot, ".tencent-channel-cli-*")
	if err != nil {
		return "", fmt.Errorf("create temporary Tencent Channel CLI: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o700); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("secure temporary Tencent Channel CLI: %w", err)
	}
	if _, err := temporary.Write(binary); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("write temporary Tencent Channel CLI: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("sync temporary Tencent Channel CLI: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close temporary Tencent Channel CLI: %w", err)
	}
	if err := os.Rename(temporaryPath, binaryPath); err != nil {
		return "", fmt.Errorf("install Tencent Channel CLI: %w", err)
	}
	digest := sha256.Sum256(binary)
	markerData, err := json.Marshal(tencentInstallMarker{PackageIntegrity: packageInfo.Integrity, BinarySHA256: hex.EncodeToString(digest[:])})
	if err != nil {
		return "", fmt.Errorf("encode Tencent Channel CLI install marker: %w", err)
	}
	if err := writeTencentInstallMarker(markerPath, markerData); err != nil {
		_ = os.Remove(binaryPath)
		return "", fmt.Errorf("write Tencent Channel CLI install marker: %w", err)
	}
	return binaryPath, nil
}

func writeTencentInstallMarker(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".tencent-channel-cli-marker-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func tencentHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Minute, CheckRedirect: tencentRedirectPolicy}
}

func tencentRedirectPolicy(next *http.Request, _ []*http.Request) error {
	if next == nil {
		return errors.New("reject Tencent Channel CLI redirect")
	}
	parsed := next.URL
	if parsed == nil || parsed.Scheme != "https" || parsed.Host != "registry.npmjs.org" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return errors.New("reject Tencent Channel CLI redirect")
	}
	return nil
}

func validInstalledTencentCLI(binaryPath, markerPath, expectedIntegrity string) (bool, error) {
	markerData, err := os.ReadFile(markerPath)
	if err != nil {
		return false, err
	}
	var marker tencentInstallMarker
	if err := json.Unmarshal(markerData, &marker); err != nil || marker.PackageIntegrity != expectedIntegrity {
		return false, nil
	}
	info, err := os.Stat(binaryPath)
	if err != nil || info.IsDir() || info.Size() <= 0 || info.Size() > maxTencentPackage {
		return false, nil
	}
	binary, err := os.ReadFile(binaryPath)
	if err != nil {
		return false, err
	}
	digest := sha256.Sum256(binary)
	return strings.EqualFold(marker.BinarySHA256, hex.EncodeToString(digest[:])), nil
}

func verifyTencentIntegrity(data []byte, integrity string) error {
	prefix, encoded, ok := strings.Cut(strings.TrimSpace(integrity), "-")
	if !ok || prefix != "sha512" {
		return errors.New("Tencent Channel CLI has an invalid pinned integrity")
	}
	want, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(want) != sha512.Size {
		return errors.New("Tencent Channel CLI has an invalid pinned SHA-512 integrity")
	}
	actual := sha512.Sum512(data)
	if string(actual[:]) != string(want) {
		return errors.New("Tencent Channel CLI package integrity does not match its pinned digest")
	}
	return nil
}

func extractTencentBinary(archive []byte, expectedPath string) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open Tencent Channel CLI package: %w", err)
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read Tencent Channel CLI package: %w", err)
		}
		if header.Name != expectedPath {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > maxTencentPackage {
			return nil, errors.New("Tencent Channel CLI package contains an invalid binary entry")
		}
		binary, err := io.ReadAll(io.LimitReader(tarReader, maxTencentPackage+1))
		if err != nil || len(binary) > maxTencentPackage {
			return nil, errors.New("read Tencent Channel CLI binary failed")
		}
		return binary, nil
	}
	return nil, fmt.Errorf("Tencent Channel CLI package is missing %s", expectedPath)
}
