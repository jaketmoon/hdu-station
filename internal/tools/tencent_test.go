package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCourseToolsOnlyReturnVisibleFieldsAndReadSearchedPosts(t *testing.T) {
	calls := []string{}
	client := &Client{run: func(ctx context.Context, args ...string) (json.RawMessage, error) {
		command := strings.Join(args, " ")
		calls = append(calls, command)
		if strings.Contains(command, "search-guild-feeds") {
			return json.RawMessage(`{"guild_feeds":[{"feed_id":"feed1","title":"好课推荐","create_time":"2026-08-01","comment_count":1,"author_id":"identity-sentinel","token":"secret-sentinel"}]}`), nil
		}
		if strings.Contains(command, "get-feed-detail") {
			return json.RawMessage(`{"feed":{"title":"好课推荐","content":"艺术鉴赏，期末小论文。","share_url":"https://pd.qq.com/s/example","author_id":"identity-sentinel"}}`), nil
		}
		if strings.Contains(command, "get-feed-comments") {
			return json.RawMessage(`{"comments":[{"content":{"text":"作业不多"},"replies_preview":[{"content_text":"不同老师考核不同"}]}],"has_more":false}`), nil
		}
		return nil, errors.New("unexpected command")
	}}
	s := NewSession(client, []string{"test-scope"}, nil)
	s.connectionChecks = map[string]func(context.Context) string{"qq": func(context.Context) string { return "ready" }}
	invalid, err := s.Read(context.Background(), ReadInput{Posts: []string{"made-up"}})
	if err != nil || len(invalid.Warnings) == 0 || len(calls) != 0 {
		t.Fatal("forged reference reached CLI")
	}
	search, err := s.Search(context.Background(), SearchInput{Query: "通识选修"})
	if err != nil || len(search.Posts) != 1 {
		t.Fatal("search failed")
	}
	read, err := s.Read(context.Background(), ReadInput{Posts: []string{search.Posts[0].ID}})
	if err != nil || len(read.Posts) != 1 || len(read.Posts[0].Discussion) != 2 {
		t.Fatal("discussion was lost")
	}
	for _, result := range []any{search, read} {
		data, _ := json.Marshal(result)
		for _, secret := range []string{"secret-sentinel", "identity-sentinel", "test-scope", "feed1"} {
			if strings.Contains(string(data), secret) {
				t.Fatal("internal data crossed tool boundary")
			}
		}
	}
	count := len(calls)
	_, _ = s.Read(context.Background(), ReadInput{Posts: []string{search.Posts[0].ID}})
	if len(calls) != count {
		t.Fatal("duplicate post was read twice")
	}
	if len(calls) != 3 {
		t.Fatalf("unexpected operations: %d", len(calls))
	}
}
func TestPartialChannelsStayUsefulAndEmptyIsNotFailure(t *testing.T) {
	client := &Client{run: func(_ context.Context, args ...string) (json.RawMessage, error) {
		if strings.Contains(strings.Join(args, " "), "unavailable") {
			return nil, errors.New("频道暂时不可用")
		}
		return json.RawMessage(`{"guild_feeds":[]}`), nil
	}}
	s := NewSession(client, []string{"available", "unavailable"}, nil)
	s.connectionChecks = map[string]func(context.Context) string{"qq": func(context.Context) string { return "ready" }}
	result, err := s.Search(context.Background(), SearchInput{Query: "课程"})
	if err != nil || len(result.Warnings) != 1 || len(result.Posts) != 0 {
		t.Fatal("failure was treated as empty evidence")
	}
}
func TestNoCLIForFlagInjectionAndUnsupportedPlatform(t *testing.T) {
	client := &Client{run: func(context.Context, ...string) (json.RawMessage, error) {
		t.Fatal("unexpected execution")
		return nil, nil
	}}
	result, _ := NewSession(client, []string{"scope"}, nil).Search(context.Background(), SearchInput{Query: "--help"})
	if len(result.Warnings) == 0 {
		t.Fatal("invalid query accepted")
	}
	if _, err := tencentPackageFor("plan9", "amd64"); err == nil {
		t.Fatal("unsupported platform silently accepted")
	}
	if got := NewClient(t.TempDir()).Status(context.Background()); got != "not_installed" {
		t.Fatalf("missing binary status = %s", got)
	}
}
