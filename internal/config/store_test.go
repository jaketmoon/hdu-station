package config

import (
	"os"
	"path/filepath"
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
