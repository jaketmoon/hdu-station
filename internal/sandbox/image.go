package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type sandboxImageManifest struct {
	Platform string `json:"platform"`
	Archive  string `json:"archive,omitempty"`
	Disk     string `json:"disk,omitempty"`
	SHA256   string `json:"sha256"`
}

func readSandboxImageManifest(dataRoot string) (sandboxImageManifest, error) {
	if strings.TrimSpace(dataRoot) == "" {
		return sandboxImageManifest{}, errors.New("sandbox data root is required")
	}
	data, err := os.ReadFile(filepath.Join(dataRoot, "sandbox", "image.json"))
	if err != nil {
		return sandboxImageManifest{}, fmt.Errorf("read sandbox image manifest: %w", err)
	}
	var manifest sandboxImageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return sandboxImageManifest{}, fmt.Errorf("decode sandbox image manifest: %w", err)
	}
	if len(manifest.SHA256) != sha256.Size*2 {
		return sandboxImageManifest{}, errors.New("sandbox image manifest has an invalid SHA-256 digest")
	}
	if _, err := hex.DecodeString(manifest.SHA256); err != nil {
		return sandboxImageManifest{}, errors.New("sandbox image manifest has an invalid SHA-256 digest")
	}
	return manifest, nil
}

func verifySandboxArtifact(dataRoot, relativePath, platform, expectedSHA256 string) (string, error) {
	if filepath.IsAbs(relativePath) || strings.TrimSpace(relativePath) == "" {
		return "", errors.New("sandbox artifact path must be relative to the Station sandbox directory")
	}
	if platform != "" && platform != expectedSandboxPlatform() {
		return "", fmt.Errorf("sandbox image targets %s, current platform is %s", platform, expectedSandboxPlatform())
	}
	sandboxRoot := filepath.Join(dataRoot, "sandbox")
	artifact := filepath.Join(sandboxRoot, relativePath)
	relative, err := filepath.Rel(sandboxRoot, artifact)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("sandbox artifact escapes the Station sandbox directory")
	}
	if err := rejectSandboxArtifactSymlinks(sandboxRoot, artifact); err != nil {
		return "", err
	}
	file, err := os.Open(artifact)
	if err != nil {
		return "", fmt.Errorf("open sandbox artifact: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash sandbox artifact: %w", err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expectedSHA256) {
		return "", errors.New("sandbox artifact digest does not match its manifest")
	}
	return artifact, nil
}

// rejectSandboxArtifactSymlinks prevents a digest check from following a
// symlink out of Station's owned sandbox directory. The manifest is local
// state and may be tampered with, so lexical containment alone is not enough.
func rejectSandboxArtifactSymlinks(sandboxRoot, artifact string) error {
	rootInfo, err := os.Lstat(sandboxRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect sandbox directory: %w", err)
	}
	if err == nil && rootInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("sandbox artifact path must not use a symbolic-link sandbox directory")
	}

	relative, err := filepath.Rel(sandboxRoot, artifact)
	if err != nil {
		return errors.New("sandbox artifact path cannot be resolved")
	}
	current := sandboxRoot
	for _, component := range splitSandboxPath(relative) {
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			break
		}
		if statErr != nil {
			return fmt.Errorf("inspect sandbox artifact path: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("sandbox artifact path must not traverse symbolic links")
		}
	}

	resolvedRoot, rootErr := filepath.EvalSymlinks(sandboxRoot)
	resolvedArtifact, artifactErr := filepath.EvalSymlinks(artifact)
	if rootErr == nil && artifactErr == nil {
		resolvedRelative, relativeErr := filepath.Rel(resolvedRoot, resolvedArtifact)
		if relativeErr != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) {
			return errors.New("sandbox artifact resolves outside the Station sandbox directory")
		}
	}
	return nil
}

func splitSandboxPath(relative string) []string {
	if relative == "." || relative == "" {
		return nil
	}
	return strings.FieldsFunc(relative, func(r rune) bool { return r == '\\' || r == '/' })
}

func expectedSandboxPlatform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
