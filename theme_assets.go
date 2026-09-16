package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/options"
)

// Serve the saved palette with the document, before CSS or settings checks load.
type themeAssets struct {
	fs.FS
	theme func() string
}

func startupPalette(theme string) (string, options.RGBA, string) {
	switch theme {
	case "teal":
		return theme, options.RGBA{R: 16, G: 25, B: 31, A: 1}, "dark"
	case "violet":
		return theme, options.RGBA{R: 21, G: 19, B: 30, A: 1}, "dark"
	case "porcelain":
		return theme, options.RGBA{R: 233, G: 237, B: 231, A: 1}, "light"
	default:
		return "harvest", options.RGBA{R: 241, G: 228, B: 199, A: 1}, "light"
	}
}

func (a *App) savedTheme() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.Appearance.Theme
}

func (a themeAssets) Open(name string) (fs.File, error) {
	if name != "index.html" {
		return a.FS.Open(name)
	}
	original, err := fs.ReadFile(a.FS, name)
	if err != nil {
		return nil, err
	}
	info, err := fs.Stat(a.FS, name)
	if err != nil {
		return nil, err
	}
	theme, color, scheme := startupPalette(a.theme())
	html := strings.Replace(string(original), `<html lang="zh-CN">`, fmt.Sprintf(`<html lang="zh-CN" data-theme="%s">`, theme), 1)
	html = strings.Replace(html, "<head>", fmt.Sprintf("<head><style>:root{background:#%02x%02x%02x;color-scheme:%s}</style>", color.R, color.G, color.B, scheme), 1)
	data := []byte(html)
	return &themeDocument{Reader: bytes.NewReader(data), info: themeDocumentInfo{FileInfo: info, size: int64(len(data))}}, nil
}

type themeDocument struct {
	*bytes.Reader
	info fs.FileInfo
}

func (f *themeDocument) Close() error               { return nil }
func (f *themeDocument) Stat() (fs.FileInfo, error) { return f.info, nil }

type themeDocumentInfo struct {
	fs.FileInfo
	size int64
}

func (i themeDocumentInfo) Size() int64 { return i.size }
