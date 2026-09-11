package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateConfigAndInvalidChangesPreserveSavedConnection(t *testing.T) {
	root := t.TempDir()
	c := Default()
	c.Model.APIKey = "private-key-sentinel"
	c.CampusKey = "campus-key-sentinel"
	if err := Save(root, c); err != nil {
		t.Fatal(err)
	}
	c.Model.BaseURL = "https://user:password@example.test"
	if err := Save(root, c); err == nil {
		t.Fatal("URL credentials accepted")
	}
	loaded, err := Load(root)
	if err != nil || loaded.Model.BaseURL != Default().Model.BaseURL {
		t.Fatal("invalid save modified connection")
	}
	data, _ := json.Marshal(loaded)
	if strings.Contains(string(data), "key-sentinel") {
		t.Fatal("key serialized in public JSON")
	}
	info, _ := os.Stat(filepath.Join(root, "config.yaml"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("config is not private")
	}
}
