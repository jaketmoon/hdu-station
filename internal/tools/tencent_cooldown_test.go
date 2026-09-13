package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jaketmoon/hdu-station/internal/config"
)

func TestQQRateLimitReturnsImmediatelyAndRecoversAfterCooldown(t *testing.T) {
	calls := 0
	c := &Client{run: func(context.Context, ...string) (json.RawMessage, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("rate_limited")
		}
		return json.RawMessage(`{"guild_feeds":[]}`), nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	for i := 0; i < 3; i++ {
		_, err := c.read(ctx, func(string) {}, "private-argument-sentinel")
		if err == nil || !strings.Contains(err.Error(), "已跳过") || strings.Contains(err.Error(), "rate_limited") || strings.Contains(err.Error(), "private-") || ctx.Err() != nil {
			t.Fatal("rate limit waited for retry or exposed internal request details")
		}
	}
	if calls != 1 || time.Until(c.cooldownUntil) < 60*time.Second {
		t.Fatal("cooldown allowed another upstream request")
	}
	// Advance just the client state instead of sleeping through the cooldown.
	c.cooldownUntil = time.Now().Add(-time.Second)
	c.next = time.Time{}
	if _, err := c.read(ctx, func(string) {}); err != nil || calls != 2 {
		t.Fatal("client did not recover after cooldown")
	}
	if time.Until(c.next) < 500*time.Millisecond {
		t.Fatal("normal 600ms pacing was removed")
	}
	ctxCancelled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := c.read(ctxCancelled, func(string) {}); !errors.Is(err, context.Canceled) || calls != 2 {
		t.Fatal("cancelled query executed")
	}
}

func TestQQCooldownDoesNotBlockOtherConnectedSource(t *testing.T) {
	xhsCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xhsCalls++
		fmt.Fprint(w, `{"success":true,"data":{"feeds":[{"id":"66abcdef1234567890abcdef","xsecToken":"private-signature-sentinel","noteCard":{"displayTitle":"课程体验"}}]}}`)
	}))
	defer server.Close()
	qqCalls := 0
	c := &Client{run: func(context.Context, ...string) (json.RawMessage, error) {
		qqCalls++
		return nil, errors.New("rate_limited")
	}}
	s := NewSession(c, []string{"scope-a", "scope-b", "scope-c"}, nil, config.Sources{Xiaohongshu: config.Xiaohongshu{Enabled: true, BaseURL: server.URL}})
	s.connectionChecks = map[string]func(context.Context) string{
		"qq":          func(context.Context) string { return "ready" },
		"xiaohongshu": func(context.Context) string { return "ready" },
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := s.Search(ctx, SearchInput{Query: "艺术"})
	if err != nil || len(result.Posts) != 1 || result.Posts[0].Source != "xiaohongshu" || qqCalls != 1 || xhsCalls != 1 {
		t.Fatal("QQ cooldown blocked healthy source or repeatedly called upstream")
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "private-") || strings.Contains(string(raw), "rate_limited") {
		t.Fatal("internal details leaked through source warnings")
	}
}
