package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/config"
)

func TestLoggedOutSourceIsProbedOnceAndDoesNotBlockQQ(t *testing.T) {
	probes, searches := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/login/status" {
			searches++
			t.Error("disconnected source searched")
		}
		probes++
		fmt.Fprint(w, `{"success":true,"data":{"is_logged_in":false}}`)
	}))
	defer server.Close()
	qqCalls := 0
	c := &Client{run: func(context.Context, ...string) (json.RawMessage, error) {
		qqCalls++
		return json.RawMessage(`{"guild_feeds":[{"feed_id":"one","title":"课程体验"}]}`), nil
	}}
	s := NewSession(c, []string{"scope"}, nil, config.Sources{Xiaohongshu: config.Xiaohongshu{Enabled: true, BaseURL: server.URL}})
	s.connectionChecks = map[string]func(context.Context) string{"qq": func(context.Context) string { return "ready" }}
	for _, q := range []string{"艺术", "音乐"} {
		r, err := s.Search(context.Background(), SearchInput{Query: q})
		if err != nil || len(r.Posts) != 1 || !strings.Contains(strings.Join(r.Warnings, ""), "未登录") {
			t.Fatal("healthy source lost with logged-out optional source")
		}
	}
	if probes != 1 || searches != 0 || qqCalls != 2 {
		t.Fatal("readiness cache or healthy search incorrect")
	}
}
