package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jaketmoon/hdu-station/internal/config"
)

// Explicit opt-in: validates one real account without using a model or writing
// anything on the platform. Reports contain counts and canonical references.
func TestLiveSourceSearchAndRead(t *testing.T) {
	source := os.Getenv("HDU_STATION_LIVE_SOURCE")
	if source == "" {
		t.Skip("set HDU_STATION_LIVE_SOURCE=zanao or xiaohongshu")
	}
	if source != "zanao" && source != "xiaohongshu" {
		t.Fatal("unsupported live source")
	}
	root, err := config.Root()
	if err != nil {
		t.Fatal("data root unavailable")
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal("configuration unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	status := ""
	if source == "zanao" {
		status = NewZanaoClient(cfg.Sources.Zanao).Status(ctx)
	} else {
		status = NewXiaohongshuClient(cfg.Sources.Xiaohongshu).Status(ctx)
	}
	t.Logf("source=%s status=%s", source, status)
	if status != "ready" {
		t.Fatal("source must be enabled and logged in before live testing")
	}
	session := NewSession(NewClient(root), nil, nil, cfg.Sources)
	query := os.Getenv("HDU_STATION_LIVE_QUERY")
	if query == "" {
		query = "选课"
	}
	found, err := session.Search(ctx, SearchInput{Source: source, Query: query})
	if err != nil {
		t.Fatal("source search did not complete")
	}
	t.Logf("search results=%d warnings=%d", len(found.Posts), len(found.Warnings))
	if len(found.Posts) == 0 {
		t.Fatal("no posts returned; inspect connection or try another course keyword")
	}
	ids := []string{}
	for _, p := range found.Posts {
		if p.Source != source {
			t.Fatal("unexpected source returned")
		}
		ids = append(ids, p.ID)
		if len(ids) == 3 {
			break
		}
	}
	read, err := session.Read(ctx, ReadInput{Posts: ids})
	if err != nil {
		t.Fatal("source reads did not complete")
	}
	t.Logf("posts read=%d warnings=%d", len(read.Posts), len(read.Warnings))
	if len(read.Posts) == 0 {
		t.Fatal("none of the searched posts could be read")
	}
	references := []map[string]any{}
	for _, p := range read.Posts {
		if p.Content == "" && p.Title == "" {
			t.Fatal("read returned no visible content")
		}
		if source == "xiaohongshu" && !IsSourceURL(p.URL) {
			t.Fatal("missing canonical post link")
		}
		if source == "zanao" && p.Locator == "" {
			t.Fatal("missing mini-program post reference")
		}
		references = append(references, map[string]any{"url": p.URL, "locator": p.Locator, "textRunes": len([]rune(p.Content)), "discussionItems": len(p.Discussion), "partial": p.Partial})
	}
	for _, payload := range []any{found, read} {
		data, _ := json.Marshal(payload)
		for _, secret := range []string{cfg.Sources.Zanao.Token, cfg.Sources.Xiaohongshu.AuthToken} {
			if secret != "" && strings.Contains(string(data), secret) {
				t.Fatal("credential reached public tool output")
			}
		}
		if strings.Contains(string(data), "xsec_token") || strings.Contains(string(data), "xsecToken") {
			t.Fatal("signature reached public tool output")
		}
	}
	report, _ := json.MarshalIndent(map[string]any{"checkedAt": time.Now().Format(time.RFC3339), "source": source, "searchResults": len(found.Posts), "readPosts": len(read.Posts), "searchWarnings": len(found.Warnings), "readWarnings": len(read.Warnings), "references": references}, "", "  ")
	dir := filepath.Join(root, "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal("cannot create report directory")
	}
	if err := os.WriteFile(filepath.Join(dir, "live-source-"+source+".json"), report, 0600); err != nil {
		t.Fatal("cannot save diagnostic report")
	}
}
