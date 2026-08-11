package appdata

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestResolveRootUsesLocalAppDataOnWindows(t *testing.T) {
	root, err := resolveRoot(
		"windows",
		func(key string) string {
			if key == "LOCALAPPDATA" {
				return `C:\Users\student\AppData\Local`
			}
			return ""
		},
		func() (string, error) { return "", errors.New("must not be called") },
	)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(`C:\Users\student\AppData\Local`, directoryName)
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}

func TestResolveRootUsesPlatformConfigDirectory(t *testing.T) {
	root, err := resolveRoot(
		"darwin",
		func(string) string { return "" },
		func() (string, error) { return "/Users/student/Library/Application Support", nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "/Users/student/Library/Application Support/HDU Station"
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}
