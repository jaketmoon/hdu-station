package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadOrCreatePersistsPrivateYAML(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	seed := FromEnvironment(func(key string) string {
		values := map[string]string{
			"HDU_STATION_CAMPUS_KEY":        "campus-test-key",
			"HDU_STATION_OPENAI_API_KEY":    "openai-test-key",
			"HDU_STATION_OPENAI_MODEL":      "gpt-test",
			"HDU_STATION_ANTHROPIC_API_KEY": "anthropic-test-key",
			"HDU_STATION_ANTHROPIC_MODEL":   "claude-test",
		}
		return values[key]
	})

	created, err := store.LoadOrCreate(seed)
	if err != nil {
		t.Fatal(err)
	}
	if created.Models.Default != "openai" {
		t.Fatalf("default provider = %q", created.Models.Default)
	}
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}

	reloaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Campus.Key != "campus-test-key" {
		t.Fatal("campus key was not persisted")
	}
	status := reloaded.Status()
	if !status.CampusConfigured || len(status.ConfiguredProviders) != 2 {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestLoadTightensExistingConfigPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix config permission bits")
	}
	root := t.TempDir()
	store := NewStore(root)
	if err := store.Save(Default()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.Path(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode after load = %o, want 600", info.Mode().Perm())
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	if err := os.WriteFile(
		filepath.Join(root, "config.yaml"),
		[]byte("version: 1\nmodels:\n  providers: {}\ncampus: {}\nweb_search:\n  provider: duckduckgo\nunexpected: true\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	_, err := store.Load()
	if err == nil || !strings.Contains(err.Error(), "field unexpected not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRequiresProviderSpecificSearchKey(t *testing.T) {
	cfg := Default()
	cfg.WebSearch.Provider = "brave"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "brave_api_key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCampusAuthStatusNeverClaimsStationDeviceAuthorization(t *testing.T) {
	withoutKey := Default().CampusAuthStatus()
	if withoutKey.State != "not_configured" || withoutKey.Configured || withoutKey.Method != "none" || !withoutKey.ServerClientRegistration {
		t.Fatalf("unexpected unconfigured campus auth status: %#v", withoutKey)
	}
	withKey := Default()
	withKey.Campus.Key = "campus-test-key"
	status := withKey.CampusAuthStatus()
	if status.State != "pat_configured_unverified" || !status.Configured || status.Method != "pat" || status.DeviceAuthorization != "unavailable" {
		t.Fatalf("unexpected configured campus auth status: %#v", status)
	}
	if strings.Contains(status.Notice, withKey.Campus.Key) {
		t.Fatal("campus auth status must not echo the campus key")
	}
}

func TestCampusAuthScopeMissingIsDistinctFromRejectedKey(t *testing.T) {
	cfg := Default()
	cfg.Campus.Key = "campus-test-key"
	status := cfg.CampusAuthStatus()
	status.State = CampusAuthScopeMissing
	if status.State == CampusAuthPATRejected {
		t.Fatal("scope-missing state must not be classified as a rejected key")
	}
}

func TestValidateSandboxImageRequiresPinnedHTTPSSource(t *testing.T) {
	for _, test := range []struct {
		name string
		url  string
		sha  string
		want string
	}{
		{name: "missing digest", url: "https://images.example.test/runtime.raw", want: "provided together"},
		{name: "insecure URL", url: "http://images.example.test/runtime.raw", sha: strings.Repeat("a", 64), want: "HTTPS URL"},
		{name: "query URL", url: "https://images.example.test/runtime.raw?token=secret", sha: strings.Repeat("a", 64), want: "without userinfo, query, or fragment"},
		{name: "bad digest", url: "https://images.example.test/runtime.raw", sha: "not-a-digest", want: "64-character hexadecimal"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := Default()
			cfg.Sandbox.ImageURL = test.url
			cfg.Sandbox.ImageSHA256 = test.sha
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestFromEnvironmentSeedsSandboxImageSettings(t *testing.T) {
	cfg := FromEnvironment(func(key string) string {
		values := map[string]string{
			"HDU_STATION_SANDBOX_IMAGE_URL":    "https://images.example.test/runtime.raw",
			"HDU_STATION_SANDBOX_IMAGE_SHA256": strings.Repeat("b", 64),
		}
		return values[key]
	})
	if cfg.Sandbox.ImageURL == "" || cfg.Sandbox.ImageSHA256 != strings.Repeat("b", 64) {
		t.Fatalf("sandbox image was not seeded from environment: %#v", cfg.Sandbox)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("environment-seeded config should validate: %v", err)
	}
}

func TestValidateSandboxImageSignatureRequiresEd25519Pair(t *testing.T) {
	missingImage := Default()
	missingImage.Sandbox.ImageSignature = base64.StdEncoding.EncodeToString(make([]byte, 64))
	if err := missingImage.Validate(); err == nil || !strings.Contains(err.Error(), "image_url and sandbox.image_sha256") {
		t.Fatalf("signature without image source was accepted: %v", err)
	}

	cfg := Default()
	cfg.Sandbox.ImageURL = "https://images.example.test/runtime.raw"
	cfg.Sandbox.ImageSHA256 = strings.Repeat("e", 64)
	cfg.Sandbox.ImageSignature = base64.StdEncoding.EncodeToString(make([]byte, 64))
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "signature and sandbox.image_public_key") {
		t.Fatalf("unexpected signature pairing error: %v", err)
	}
	cfg.Sandbox.ImagePublicKey = base64.StdEncoding.EncodeToString(make([]byte, 32))
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid-shaped signature configuration rejected: %v", err)
	}
}
