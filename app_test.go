package main

import (
	"runtime"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/config"
)

func TestBootstrapIdentifiesLocalDesktopBuild(t *testing.T) {
	cfg := config.Default()
	cfg.Campus.Key = "campus-test-key"
	provider := cfg.Models.Providers["openai"]
	provider.APIKey = "model-test-key"
	provider.Model = "gpt-test"
	cfg.Models.Providers["openai"] = provider
	cfg.Models.Default = "openai"
	state := NewApp("/tmp/station-test", cfg).Bootstrap()

	if state.Name != "HDU Station" {
		t.Fatalf("unexpected app name %q", state.Name)
	}
	if state.Version == "" {
		t.Fatal("version must not be empty")
	}
	wantPlatform := runtime.GOOS + "/" + runtime.GOARCH
	if state.Platform != wantPlatform {
		t.Fatalf("platform = %q, want %q", state.Platform, wantPlatform)
	}
	if state.DataRoot != "/tmp/station-test" {
		t.Fatalf("data root = %q", state.DataRoot)
	}
	if !state.Configuration.CampusConfigured || state.Configuration.DefaultProvider != "openai" {
		t.Fatalf("unexpected configuration status: %#v", state.Configuration)
	}
}
