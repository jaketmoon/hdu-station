package sandbox

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
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

// ImageSource is deliberately small: the digest is pinned by local
// configuration and the URL is only a transport location. No credentials are
// accepted here, so the image downloader cannot become a secret relay.
type ImageSource struct {
	URL       string
	SHA256    string
	Signature string
	PublicKey string
}

const maxSandboxImageBytes int64 = 16 << 30

func (source ImageSource) configured() bool {
	return strings.TrimSpace(source.URL) != "" || strings.TrimSpace(source.SHA256) != "" || strings.TrimSpace(source.Signature) != "" || strings.TrimSpace(source.PublicKey) != ""
}

func (source ImageSource) validate() error {
	imageURL := strings.TrimSpace(source.URL)
	imageSHA256 := strings.TrimSpace(source.SHA256)
	if imageURL == "" && imageSHA256 == "" {
		return nil
	}
	if imageURL == "" || imageSHA256 == "" {
		return errors.New("sandbox image URL and SHA-256 must be provided together")
	}
	parsed, err := url.Parse(imageURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return errors.New("sandbox image URL must be an HTTPS URL without userinfo, query, or fragment")
	}
	if len(imageSHA256) != sha256.Size*2 {
		return errors.New("sandbox image SHA-256 must be a 64-character hexadecimal digest")
	}
	if _, err := hex.DecodeString(imageSHA256); err != nil {
		return errors.New("sandbox image SHA-256 must be a 64-character hexadecimal digest")
	}
	signature := strings.TrimSpace(source.Signature)
	publicKey := strings.TrimSpace(source.PublicKey)
	if (signature == "") != (publicKey == "") {
		return errors.New("sandbox image signature and public key must be provided together")
	}
	if signature != "" {
		decodedSignature, signatureErr := base64.StdEncoding.DecodeString(signature)
		decodedPublicKey, publicKeyErr := base64.StdEncoding.DecodeString(publicKey)
		if signatureErr != nil || len(decodedSignature) != ed25519.SignatureSize || publicKeyErr != nil || len(decodedPublicKey) != ed25519.PublicKeySize {
			return errors.New("sandbox image signature and public key must be base64 Ed25519 values")
		}
	}
	return nil
}

// ensureSandboxImage keeps the manifest and its digest-named artifact
// installable as a single logical state. A manifest is replaced atomically
// only after the complete artifact has been downloaded and verified.
func ensureSandboxImage(ctx context.Context, dataRoot string, source ImageSource, platform, kind string) (sandboxImageManifest, error) {
	return ensureSandboxImageWithClient(ctx, dataRoot, source, platform, kind, sandboxImageHTTPClient())
}

func ensureSandboxImageWithClient(ctx context.Context, dataRoot string, source ImageSource, platform, kind string, client *http.Client) (sandboxImageManifest, error) {
	if !source.configured() {
		return readSandboxImageManifest(dataRoot)
	}
	if err := source.validate(); err != nil {
		return sandboxImageManifest{}, err
	}
	if strings.TrimSpace(dataRoot) == "" {
		return sandboxImageManifest{}, errors.New("sandbox data root is required")
	}
	if kind != "disk" && kind != "archive" {
		return sandboxImageManifest{}, fmt.Errorf("unsupported sandbox image kind %q", kind)
	}

	digest := strings.ToLower(strings.TrimSpace(source.SHA256))
	relativeArtifact := filepath.ToSlash(filepath.Join("images", "runtime."+kind+"-"+digest))
	manifest := sandboxImageManifest{Platform: platform, SHA256: digest}
	if kind == "disk" {
		manifest.Disk = relativeArtifact
	} else {
		manifest.Archive = relativeArtifact
	}

	if current, err := readSandboxImageManifest(dataRoot); err == nil && current.Platform == manifest.Platform && current.SHA256 == manifest.SHA256 && current.Disk == manifest.Disk && current.Archive == manifest.Archive {
		if _, err := verifySandboxArtifact(dataRoot, relativeArtifact, platform, digest); err == nil && verifySandboxImageSignature(source, digest) == nil {
			return current, nil
		}
	}

	sandboxRoot := filepath.Join(dataRoot, "sandbox")
	imageRoot := filepath.Join(sandboxRoot, "images")
	artifactPath := filepath.Join(sandboxRoot, filepath.FromSlash(relativeArtifact))
	// Validate the complete destination before creating directories. A local
	// manifest or a tampered data root must not redirect MkdirAll/downloads to
	// a location outside Station's sandbox directory.
	if err := rejectSandboxArtifactSymlinks(sandboxRoot, artifactPath); err != nil {
		return sandboxImageManifest{}, err
	}
	if err := os.MkdirAll(imageRoot, 0o700); err != nil {
		return sandboxImageManifest{}, fmt.Errorf("create sandbox image directory: %w", err)
	}
	if _, err := verifySandboxArtifact(dataRoot, relativeArtifact, platform, digest); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			// Never remove an invalid artifact through a symlinked parent. The
			// manifest and image directory are local state, so fail closed if
			// either was redirected outside Station's owned root.
			if symlinkErr := rejectSandboxArtifactSymlinks(sandboxRoot, artifactPath); symlinkErr != nil {
				return sandboxImageManifest{}, symlinkErr
			}
			// A digest-named artifact that is present but corrupt is owned by
			// Station. Remove only that exact artifact before re-fetching it.
			if removeErr := os.Remove(artifactPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				return sandboxImageManifest{}, fmt.Errorf("replace invalid sandbox image: %w", removeErr)
			}
		}
		if err := downloadSandboxArtifact(ctx, source, artifactPath, client); err != nil {
			return sandboxImageManifest{}, err
		}
	}
	if _, err := verifySandboxArtifact(dataRoot, relativeArtifact, platform, digest); err != nil {
		return sandboxImageManifest{}, err
	}
	if err := verifySandboxImageSignature(source, digest); err != nil {
		return sandboxImageManifest{}, err
	}
	if err := writeSandboxImageManifest(dataRoot, manifest); err != nil {
		return sandboxImageManifest{}, err
	}
	return manifest, nil
}

func downloadSandboxArtifact(ctx context.Context, source ImageSource, artifactPath string, client *http.Client) error {
	if ctx == nil {
		ctx = context.Background()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(source.URL), nil)
	if err != nil {
		return fmt.Errorf("create sandbox image request: %w", err)
	}
	if client == nil {
		client = sandboxImageHTTPClient()
	} else {
		copy := *client
		copy.CheckRedirect = sandboxImageRedirectPolicy
		client = &copy
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download sandbox image: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("download sandbox image: server returned HTTP %d", response.StatusCode)
	}

	temporary, err := os.CreateTemp(filepath.Dir(artifactPath), ".sandbox-image-*")
	if err != nil {
		return fmt.Errorf("create temporary sandbox image: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary sandbox image: %w", err)
	}

	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, maxSandboxImageBytes+1))
	if copyErr != nil {
		_ = temporary.Close()
		return fmt.Errorf("write sandbox image: %w", copyErr)
	}
	if written > maxSandboxImageBytes {
		_ = temporary.Close()
		return fmt.Errorf("sandbox image exceeds the %d-byte limit", maxSandboxImageBytes)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary sandbox image: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary sandbox image: %w", err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, source.SHA256) {
		return errors.New("sandbox image digest does not match configured SHA-256")
	}
	if err := verifySandboxImageSignature(source, actual); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, artifactPath); err != nil {
		return fmt.Errorf("install sandbox image: %w", err)
	}
	return nil
}

func verifySandboxImageSignature(source ImageSource, digest string) error {
	if strings.TrimSpace(source.Signature) == "" && strings.TrimSpace(source.PublicKey) == "" {
		return nil
	}
	if err := source.validate(); err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(source.Signature))
	if err != nil {
		return errors.New("decode sandbox image signature")
	}
	publicKey, err := base64.StdEncoding.DecodeString(strings.TrimSpace(source.PublicKey))
	if err != nil {
		return errors.New("decode sandbox image public key")
	}
	digestBytes, err := hex.DecodeString(strings.TrimSpace(digest))
	if err != nil || len(digestBytes) != sha256.Size {
		return errors.New("sandbox image signature digest is invalid")
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), digestBytes, signature) {
		return errors.New("sandbox image signature does not match configured SHA-256")
	}
	return nil
}

func sandboxImageHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Minute, CheckRedirect: sandboxImageRedirectPolicy}
}

func sandboxImageRedirectPolicy(next *http.Request, _ []*http.Request) error {
	if err := validateSandboxImageURL(next.URL); err != nil {
		return fmt.Errorf("reject sandbox image redirect: %w", err)
	}
	return nil
}

func validateSandboxImageURL(parsed *url.URL) error {
	if parsed == nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return errors.New("sandbox image URL must be an HTTPS URL without userinfo, query, or fragment")
	}
	return nil
}

func writeSandboxImageManifest(dataRoot string, manifest sandboxImageManifest) error {
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode sandbox image manifest: %w", err)
	}
	manifestPath := filepath.Join(dataRoot, "sandbox", "image.json")
	temporary, err := os.CreateTemp(filepath.Dir(manifestPath), ".sandbox-manifest-*")
	if err != nil {
		return fmt.Errorf("create temporary sandbox image manifest: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary sandbox image manifest: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary sandbox image manifest: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary sandbox image manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary sandbox image manifest: %w", err)
	}
	if err := os.Rename(temporaryPath, manifestPath); err != nil {
		return fmt.Errorf("install sandbox image manifest: %w", err)
	}
	return nil
}
