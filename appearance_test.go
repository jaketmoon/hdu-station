package main

import (
	"path/filepath"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/config"
)

func TestAppearanceSavesDuringTurnWithoutChangingCredentials(t *testing.T) {
	a := testApp(t)
	before := a.cfg
	a.active["display-test"] = &activeTurn{}
	next := config.Appearance{InstantText: true, Theme: "porcelain"}
	saved, err := a.SaveAppearance(next)
	delete(a.active, "display-test")
	if err != nil || saved != next {
		t.Fatal("display preferences could not be saved during a turn")
	}
	loaded, err := config.Load(a.root)
	if err != nil || loaded.Appearance != next || loaded.Model != before.Model || loaded.Sources != before.Sources || loaded.CampusKey != before.CampusKey {
		t.Fatal("display save failed to preserve existing connection configuration")
	}
}

func TestAppearanceFailurePreservesCurrentPreferences(t *testing.T) {
	a := testApp(t)
	before := a.cfg.Appearance
	root := a.root
	a.root = filepath.Join(root, "config.yaml")
	_, err := a.SaveAppearance(config.Appearance{InstantText: true, Theme: "porcelain"})
	a.root = root
	if err == nil || a.cfg.Appearance != before {
		t.Fatal("failed display save changed the in-memory preference")
	}
}

func TestInvalidThemePreservesSavedAppearance(t *testing.T) {
	a := testApp(t)
	before := config.Appearance{InstantText: true, Theme: "violet"}
	if _, err := a.SaveAppearance(before); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveAppearance(config.Appearance{Theme: "unknown"}); err == nil {
		t.Fatal("unsupported theme accepted")
	}
	loaded, err := config.Load(a.root)
	if err != nil || loaded.Appearance != before || a.cfg.Appearance != before {
		t.Fatal("invalid theme changed the saved appearance")
	}
}
