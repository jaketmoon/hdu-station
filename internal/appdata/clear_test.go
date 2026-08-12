package appdata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClearRootOnlyRemovesNamedStationRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, directoryName)
	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "keep.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ClearRoot(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("Station root still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, "keep.txt")); err != nil {
		t.Fatalf("unrelated file was removed: %v", err)
	}
}

func TestClearRootRejectsBroadOrUnrelatedPaths(t *testing.T) {
	for _, root := range []string{"", ".", string(filepath.Separator), filepath.Join(t.TempDir(), "other")} {
		if err := ClearRoot(root); err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Fatalf("ClearRoot(%q) error = %v", root, err)
		}
	}
}
