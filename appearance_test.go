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
	next := config.Appearance{InstantText: true}
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
	root := a.root
	a.root = filepath.Join(root, "config.yaml")
	_, err := a.SaveAppearance(config.Appearance{InstantText: true})
	a.root = root
	if err == nil || a.cfg.Appearance.InstantText {
		t.Fatal("failed display save changed the in-memory preference")
	}
}
