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
	c.Sources.Zanao.Token = "zanao-key-sentinel"
	c.Sources.Xiaohongshu.AuthToken = "xhs-key-sentinel"
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

func TestSourceConfigValidationAndLegacyDefaults(t *testing.T) {
	for _, address := range []string{"http://127.0.0.1:18060", "http://localhost:18060", "http://[::1]:18060/"} {
		if err := ValidateXiaohongshuURL(address); err != nil {
			t.Fatal("valid local service rejected")
		}
	}
	for _, address := range []string{"https://remote.example:18060", "http://127.0.0.1:18060/mcp", "http://user:secret@127.0.0.1:18060", "http://127.0.0.1:18060?token=secret", "http://127.0.0.1:18060#x", "http://localhost", "http://localhost:0", "http://localhost:99999", "file:///tmp/socket", "http://127.0.0.1.evil.test:18060"} {
		if ValidateXiaohongshuURL(address) == nil {
			t.Fatal("unsafe service address accepted")
		}
	}
	for _, sources := range []Sources{
		{Zanao: Zanao{Enabled: true}},
		{Zanao: Zanao{Enabled: true, SchoolAlias: "hdu"}},
		{Zanao: Zanao{SchoolAlias: "hdu\nheader"}},
		{Zanao: Zanao{Token: "token\nheader"}},
		{Xiaohongshu: Xiaohongshu{AuthToken: "token\rheader"}},
	} {
		if sources.Validate() == nil {
			t.Fatal("invalid source configuration accepted")
		}
	}
	root := t.TempDir()
	path := filepath.Join(root, "config.yaml")
	legacy := "version: 1\nmodel:\n  base_url: https://api.deepseek.com\n  name: deepseek-flash\n  api_key: private-sentinel\n"
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil || cfg.Sources.Zanao.Enabled || cfg.Sources.Xiaohongshu.Enabled || cfg.Sources.Xiaohongshu.BaseURL != DefaultXiaohongshuURL {
		t.Fatal("legacy configuration did not keep optional sources disabled")
	}
	for _, value := range []string{"{}", "version: 999"} {
		_ = os.WriteFile(path, []byte(value), 0600)
		if _, err := Load(root); err == nil {
			t.Fatal("invalid version accepted")
		}
	}
}
