package appdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ClearRoot removes only the exact Station-owned application-data directory.
// It intentionally does not reach into the user's home directory or external
// connector-owned credential stores.
func ClearRoot(root string) error {
	cleaned := filepath.Clean(strings.TrimSpace(root))
	if cleaned == "." || cleaned == string(filepath.Separator) || cleaned == "" {
		return errors.New("refusing to clear an unsafe application data root")
	}
	if filepath.Base(cleaned) != directoryName {
		return fmt.Errorf("refusing to clear non-Station data root %q", cleaned)
	}
	if err := os.RemoveAll(cleaned); err != nil {
		return fmt.Errorf("clear Station application data: %w", err)
	}
	return nil
}
