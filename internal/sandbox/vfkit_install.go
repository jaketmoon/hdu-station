//go:build darwin

package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	vfkitVersion       = "v0.6.4"
	maxVFKitBinarySize = 128 << 20
)

type vfkitPackage struct {
	URL    string
	SHA256 string
}

type vfkitInstallMarker struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

var pinnedVFKitPackage = vfkitPackage{
	URL:    "https://github.com/crc-org/vfkit/releases/download/v0.6.4/vfkit",
	SHA256: "0ed83fc8ca7aa708598835480dba1362406aa7cd1dab3b27464eb76327d9652d",
}

func resolveVFKitPath(dataRoot string) (string, string) {
	if strings.TrimSpace(dataRoot) != "" {
		path := filepath.Join(dataRoot, "tools", "vfkit", vfkitVersion, vfkitExecutable)
		if valid, err := validVFKitInstall(path, filepath.Join(filepath.Dir(path), "install.json"), pinnedVFKitPackage); err == nil && valid {
			return path, "station"
		}
	}
	return "", ""
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

func ensureVFKit(ctx context.Context, dataRoot string, client *http.Client) (string, error) {
	return ensureVFKitWithPackage(ctx, dataRoot, pinnedVFKitPackage, client)
}

func ensureVFKitWithPackage(ctx context.Context, dataRoot string, packageInfo vfkitPackage, client *http.Client) (string, error) {
	if strings.TrimSpace(dataRoot) == "" {
		return "", errors.New("Sandbox data root is required")
	}
	if err := validateVFKitPackage(packageInfo); err != nil {
		return "", err
	}
	installRoot := filepath.Join(dataRoot, "tools", "vfkit", vfkitVersion)
	binaryPath := filepath.Join(installRoot, vfkitExecutable)
	markerPath := filepath.Join(installRoot, "install.json")
	if valid, err := validVFKitInstall(binaryPath, markerPath, packageInfo); err == nil && valid {
		return binaryPath, nil
	}
	if client == nil {
		client = vfkitHTTPClient()
	} else {
		copy := *client
		copy.CheckRedirect = vfkitRedirectPolicy
		client = &copy
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, packageInfo.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create vfkit download request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("download vfkit: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("download vfkit: server returned HTTP %d", response.StatusCode)
	}
	if err := os.MkdirAll(installRoot, 0o700); err != nil {
		return "", fmt.Errorf("create vfkit directory: %w", err)
	}
	temporary, err := os.CreateTemp(installRoot, ".vfkit-*")
	if err != nil {
		return "", fmt.Errorf("create temporary vfkit: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o700); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("secure temporary vfkit: %w", err)
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, maxVFKitBinarySize+1))
	if err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("write temporary vfkit: %w", err)
	}
	if written > maxVFKitBinarySize {
		_ = temporary.Close()
		return "", errors.New("vfkit binary exceeds the 128 MiB limit")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("sync temporary vfkit: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close temporary vfkit: %w", err)
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(digest, packageInfo.SHA256) {
		return "", errors.New("vfkit binary digest does not match the pinned release")
	}
	if err := os.Rename(temporaryPath, binaryPath); err != nil {
		return "", fmt.Errorf("install vfkit: %w", err)
	}
	marker, err := json.Marshal(vfkitInstallMarker{Version: vfkitVersion, SHA256: strings.ToLower(packageInfo.SHA256)})
	if err != nil {
		_ = os.Remove(binaryPath)
		return "", fmt.Errorf("encode vfkit install marker: %w", err)
	}
	if err := writeVFKitInstallMarker(markerPath, marker); err != nil {
		_ = os.Remove(binaryPath)
		return "", fmt.Errorf("write vfkit install marker: %w", err)
	}
	return binaryPath, nil
}

func writeVFKitInstallMarker(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".vfkit-marker-*")
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

func validateVFKitPackage(packageInfo vfkitPackage) error {
	parsed, err := url.Parse(packageInfo.URL)
	if err != nil || packageInfo.URL != pinnedVFKitPackage.URL || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("vfkit package URL must be the fixed official GitHub HTTPS URL")
	}
	if len(packageInfo.SHA256) != sha256.Size*2 {
		return errors.New("vfkit package has an invalid pinned SHA-256 digest")
	}
	if _, err := hex.DecodeString(packageInfo.SHA256); err != nil {
		return errors.New("vfkit package has an invalid pinned SHA-256 digest")
	}
	return nil
}

func validVFKitInstall(binaryPath, markerPath string, packageInfo vfkitPackage) (bool, error) {
	markerData, err := os.ReadFile(markerPath)
	if err != nil {
		return false, err
	}
	var marker vfkitInstallMarker
	if err := json.Unmarshal(markerData, &marker); err != nil || marker.Version != vfkitVersion || !strings.EqualFold(marker.SHA256, packageInfo.SHA256) {
		return false, nil
	}
	if !isExecutableFile(binaryPath) {
		return false, nil
	}
	info, err := os.Stat(binaryPath)
	if err != nil || info.Size() <= 0 || info.Size() > maxVFKitBinarySize {
		return false, nil
	}
	binary, err := os.ReadFile(binaryPath)
	if err != nil {
		return false, err
	}
	digest := sha256.Sum256(binary)
	return strings.EqualFold(hex.EncodeToString(digest[:]), packageInfo.SHA256), nil
}

func vfkitHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Minute, CheckRedirect: vfkitRedirectPolicy}
}

func vfkitRedirectPolicy(next *http.Request, _ []*http.Request) error {
	if next == nil || next.URL.Scheme != "https" || next.URL.User != nil || next.URL.Fragment != "" {
		return errors.New("reject vfkit redirect")
	}
	// GitHub release downloads are short-lived, signed Azure URLs. The initial
	// URL remains hard-pinned above; only GitHub's release-asset host may be
	// followed and it necessarily carries a query string with the signature.
	if !strings.EqualFold(next.URL.Host, "release-assets.githubusercontent.com") || strings.TrimSpace(next.URL.RawQuery) == "" {
		return errors.New("reject vfkit redirect")
	}
	return nil
}
