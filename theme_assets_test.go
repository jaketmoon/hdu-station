package main

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStartupThemeDocument(t *testing.T) {
	selected := "teal"
	assets := themeAssets{FS: fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<html lang="zh-CN"><head></head><body></body></html>`)},
		"app.js":     &fstest.MapFile{Data: []byte("unchanged")},
	}, theme: func() string { return selected }}
	for _, theme := range []string{"teal", "violet", "porcelain", "harvest", `"><script>`, ""} {
		selected = theme
		data, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			t.Fatal(err)
		}
		want, _, _ := startupPalette(theme)
		if !strings.Contains(string(data), `data-theme="`+want+`"`) || strings.Contains(string(data), "<script>") {
			t.Fatalf("unexpected startup document: %s", data)
		}
		info, err := fs.Stat(assets, "index.html")
		if err != nil || info.Size() != int64(len(data)) {
			t.Fatal("incorrect document size", err)
		}
	}
	data, err := fs.ReadFile(assets, "app.js")
	if err != nil || string(data) != "unchanged" {
		t.Fatal("asset changed", err)
	}
}
