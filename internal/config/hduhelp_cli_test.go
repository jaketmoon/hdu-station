package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadHduhelpCLIPATAcceptsCurrentCredentialShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	const token = "hduhelp_pat_0123456789012345678901234567890123456789"
	if err := os.WriteFile(path, []byte(`{"server":"https://api.hduhelp.com","token":"`+token+`","expires_at":"2030-01-01T00:00:00Z","scopes":["academic:course:read"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadHduhelpCLIPAT(path, time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got != token {
		t.Fatal("credential token was not loaded")
	}
}

func TestLoadHduhelpCLIPATRejectsUnsafeOrExpiredCredential(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{"wrong-server", `{"server":"https://example.test","token":"hduhelp_pat_0123456789012345678901234567890123456789"}`, "official HDUHelp API"},
		{"bad-token", `{"server":"https://api.hduhelp.com","token":"not-a-pat"}`, "invalid personal access token"},
		{"expired", `{"server":"https://api.hduhelp.com","token":"hduhelp_pat_0123456789012345678901234567890123456789","expires_at":"2020-01-01T00:00:00Z"}`, "has expired"},
		{"multiple-values", `{"server":"https://api.hduhelp.com","token":"hduhelp_pat_0123456789012345678901234567890123456789"} {}`, "multiple JSON values"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(test.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadHduhelpCLIPAT(path, time.Now())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
