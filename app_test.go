package main

import (
	"runtime"
	"testing"
)

func TestBootstrapIdentifiesLocalDesktopBuild(t *testing.T) {
	state := NewApp().Bootstrap()

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
}
