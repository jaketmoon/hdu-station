package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/config"
)

func TestCreateApplicationSeedsAndReloadsConfiguration(t *testing.T) {
	root := t.TempDir()
	seed := config.FromEnvironment(func(key string) string {
		values := map[string]string{
			"HDU_STATION_CAMPUS_KEY":     "campus-seed",
			"HDU_STATION_OPENAI_API_KEY": "openai-seed",
			"HDU_STATION_OPENAI_MODEL":   "gpt-seed",
		}
		return values[key]
	})

	app, err := createApplicationAt(root, seed)
	if err != nil {
		t.Fatal(err)
	}
	if !app.Bootstrap().Configuration.CampusConfigured {
		t.Fatal("campus configuration should be reported as configured")
	}

	secondSeed := config.Default()
	reloaded, err := createApplicationAt(root, secondSeed)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Bootstrap().Configuration.DefaultProvider != "openai" {
		t.Fatal("existing YAML must win over a later environment seed")
	}
	if _, err := os.Stat(filepath.Join(root, "config.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestReadOptionalEnvironmentAllowsMissingFile(t *testing.T) {
	values, err := readOptionalEnvironment(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 0 {
		t.Fatalf("unexpected values: %#v", values)
	}
}
