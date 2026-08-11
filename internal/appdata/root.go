package appdata

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const directoryName = "HDU Station"

func ResolveRoot() (string, error) {
	return resolveRoot(runtime.GOOS, os.Getenv, os.UserConfigDir)
}

func resolveRoot(goos string, getenv func(string) string, userConfigDir func() (string, error)) (string, error) {
	if goos == "windows" {
		if local := getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, directoryName), nil
		}
	}
	base, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(base, directoryName), nil
}
