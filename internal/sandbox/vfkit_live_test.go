//go:build darwin

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLiveEnsurePinnedVFKit(t *testing.T) {
	if os.Getenv("HDU_STATION_RUN_LIVE_TESTS") != "1" {
		t.Skip("set HDU_STATION_RUN_LIVE_TESTS=1 to download and verify the pinned official vfkit release")
	}
	root, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	path, err := ensureVFKit(context.Background(), filepath.Join(root, "HDU Station"), vfkitHTTPClient())
	if err != nil {
		t.Fatal(err)
	}
	if valid, verifyErr := validVFKitInstall(path, filepath.Join(filepath.Dir(path), "install.json"), pinnedVFKitPackage); verifyErr != nil || !valid {
		t.Fatalf("installed vfkit failed verification: valid=%v err=%v", valid, verifyErr)
	}
}
