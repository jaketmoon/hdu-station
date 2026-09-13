package tools

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jaketmoon/hdu-station/internal/config"
)

type sourceTransport func(*http.Request) (*http.Response, error)

func (f sourceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mockZanao(t *testing.T, client *ZanaoClient, handler func(*http.Request) string) {
	t.Helper()
	client.http.Transport = sourceTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Scheme != "https" || r.URL.Host != "api.x.zanao.com" {
			t.Error("unexpected Zanao destination or method")
		}
		if r.Header.Get("X-Sc-Od") != client.cfg.Token || r.Header.Get("X-Sc-Alias") != client.cfg.SchoolAlias {
			t.Error("missing campus credentials")
		}
		sign := md5.Sum([]byte(client.cfg.SchoolAlias + "_" + r.Header.Get("X-Sc-Nd") + "_" + r.Header.Get("X-Sc-Td") + "_1b6d2514354bc407afdd935f45521a8c"))
		if r.Header.Get("X-Sc-Ah") != hex.EncodeToString(sign[:]) {
			t.Error("invalid request signature")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(handler(r))), Request: r}, nil
	})
}

func TestMixedSourcesSearchReadCacheAndCredentialIsolation(t *testing.T) {
	const noteID = "66abcdef1234567890abcdef"
	xhsCalls, zanaoCalls := 0, 0
	xhs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xhsCalls++
		if r.Header.Get("Authorization") != "Bearer service-private-sentinel" {
			t.Error("missing service auth")
		}
		var input map[string]any
		_ = json.NewDecoder(r.Body).Decode(&input)
		switch r.URL.Path {
		case "/api/v1/login/status":
			fmt.Fprint(w, `{"success":true,"data":{"is_logged_in":true}}`)
		case "/api/v1/feeds/search":
			if r.Method != "POST" || input["keyword"] != "杭电 通识选修" {
				t.Error("search lost campus scope")
			}
			fmt.Fprint(w, `{"success":true,"data":{"feeds":[{"id":"`+noteID+`","xsecToken":"signature-private-sentinel","modelType":"note","noteCard":{"displayTitle":"选修体验","user":{"userId":"author-private-sentinel"},"interactInfo":{"commentCount":"3"}}},{"id":"bad-card","modelType":"hot_query"}]}}`)
		case "/api/v1/feeds/detail":
			if input["feed_id"] != noteID || input["xsec_token"] != "signature-private-sentinel" || input["load_all_comments"] != false {
				t.Error("detail did not use host-held search identity and bounded comments")
			}
			fmt.Fprint(w, `{"success":true,"data":{"feed_id":"`+noteID+`","data":{"note":{"noteId":"`+noteID+`","title":"选修体验","desc":"期末交论文","time":1780000000000,"user":{"userId":"author-private-sentinel"}},"comments":{"list":[{"content":"认真写作业","subCommentCount":"2","subComments":[{"content":"老师会反馈"}]}],"hasMore":true}}}}`)
		default:
			t.Error("non-read endpoint reached")
			w.WriteHeader(404)
		}
	}))
	defer xhs.Close()
	cfg := config.Sources{
		Zanao:       config.Zanao{Enabled: true, SchoolAlias: "test-campus", Token: "zanao-private-sentinel"},
		Xiaohongshu: config.Xiaohongshu{Enabled: true, BaseURL: xhs.URL, AuthToken: "service-private-sentinel"},
	}
	qqCalls := 0
	qq := &Client{run: func(_ context.Context, args ...string) (json.RawMessage, error) {
		qqCalls++
		switch args[1] {
		case "search-guild-feeds":
			return json.RawMessage(`{"guild_feeds":[{"feed_id":"same-id","title":"QQ体验"}]}`), nil
		case "get-feed-detail":
			return json.RawMessage(`{"feed":{"content":"考核看老师","share_url":"https://pd.qq.com/s/course"}}`), nil
		case "get-feed-comments":
			return json.RawMessage(`{"comments":[],"has_more":false}`), nil
		}
		return nil, errors.New("unexpected operation")
	}}
	s := NewSession(qq, []string{"guild-private-sentinel"}, nil, cfg)
	s.connectionChecks = map[string]func(context.Context) string{"qq": func(context.Context) string { return "ready" }}
	mockZanao(t, s.zanao, func(r *http.Request) string {
		zanaoCalls++
		switch r.URL.Path {
		case "/user/info":
			return `{"errno":0,"data":{"school_name":"测试学校"}}`
		case "/thread/v2/search":
			if r.URL.Query().Get("wd") != "通识选修" || r.URL.Query().Get("cur_page") != "1" {
				t.Error("unexpected search params")
			}
			if r.URL.Query().Get("cate_id") == "20" {
				if r.URL.Query().Get("range") != "1y" {
					t.Error("missing history range")
				}
				return `{"errno":0,"data":{"list":[{"thread_id":"same-id","title":"重复帖子","c_count":2}]}}`
			}
			return `{"errno":0,"data":[{"thread_id":"same-id","title":"赞哦体验","c_count":"2","nickname":"author-private-sentinel"}]}`
		case "/thread/info":
			_ = r.ParseForm()
			if r.PostForm.Get("id") != "same-id" {
				t.Error("detail id not form encoded")
			}
			return `{"errno":0,"data":{"detail":{"thread_id":"same-id","content":"平时有签到","contact_phone":"phone-private-sentinel","nickname":"author-private-sentinel"}}}`
		case "/comment/list":
			if r.URL.Query().Get("id") != "same-id" {
				t.Error("wrong comment target")
			}
			return `{"errno":0,"data":{"list":[{"content":"期末论文","reply_list":[{"content":"看课程要求"}]}]}}`
		default:
			t.Error("non-read endpoint reached")
			return `{"errno":1}`
		}
	})
	ctx := context.Background()
	found, err := s.Search(ctx, SearchInput{Query: "通识选修"})
	if err != nil || len(found.Posts) != 3 || len(found.Warnings) != 0 {
		t.Fatal("mixed source search failed")
	}
	ids := []string{}
	for i, source := range []string{"qq", "zanao", "xiaohongshu"} {
		if found.Posts[i].Source != source {
			t.Fatal("source identity was lost")
		}
		ids = append(ids, found.Posts[i].ID)
	}
	read, err := s.Read(ctx, ReadInput{Posts: ids})
	if err != nil || len(read.Posts) != 3 || s.Reads != 3 {
		t.Fatal("mixed source read failed")
	}
	if read.Posts[1].URL != "" || read.Posts[1].Locator == "" || len(read.Posts[1].Discussion) != 2 || !read.Posts[1].Partial {
		t.Fatal("Zanao locator or partial comments missing")
	}
	xpost := read.Posts[2]
	if len(xpost.Discussion) != 2 || !xpost.Partial || !strings.Contains(xpost.BrowserURL(), "xsec_token=") || strings.Contains(xpost.URL, "?") {
		t.Fatal("Xiaohongshu signed and public URLs not separated")
	}
	for _, value := range []any{found, read, s.Sources()} {
		data, _ := json.Marshal(value)
		if strings.Contains(string(data), "private-sentinel") || strings.Contains(string(data), "xsecToken") {
			t.Fatal("private source data crossed public boundary")
		}
	}
	_, _ = s.Read(ctx, ReadInput{Posts: append(ids, "forged-post")})
	if xhsCalls != 3 || zanaoCalls != 5 || qqCalls != 3 || s.Reads != 3 {
		t.Fatal("duplicate or forged reference reached upstream")
	}
	again, _ := s.Search(ctx, SearchInput{Query: "通识选修", Source: "zanao"})
	if len(again.Posts) != 1 || again.Posts[0].ID != ids[1] {
		t.Fatal("repeat searches changed post identity")
	}
}

func TestSourceSelectionFailuresAndEmptyResults(t *testing.T) {
	s := NewSession(&Client{run: func(context.Context, ...string) (json.RawMessage, error) {
		t.Fatal("unexpected QQ call")
		return nil, nil
	}}, nil, nil)
	for _, source := range []string{"zanao", "xiaohongshu", "publish"} {
		result, err := s.Search(context.Background(), SearchInput{Query: "课程", Source: source})
		if err != nil || len(result.Warnings) == 0 {
			t.Fatal("disabled or invalid source silently treated as empty")
		}
	}
	s.sources.Zanao.Enabled = true
	s.connections = nil
	s.connectionChecks = map[string]func(context.Context) string{"zanao": func(context.Context) string { return "ready" }}
	s.zanao = NewZanaoClient(config.Zanao{Enabled: true, SchoolAlias: "test", Token: "private-sentinel"})
	mockZanao(t, s.zanao, func(r *http.Request) string {
		if r.URL.Query().Get("cate_id") == "10" {
			return `{"errno":1,"errmsg":"private-sentinel","data":null}`
		}
		return `{"errno":0,"data":[]}`
	})
	result, err := s.Search(context.Background(), SearchInput{Query: "课程", Source: "zanao"})
	if err != nil || len(result.Warnings) != 1 || len(result.Posts) != 0 || strings.Contains(result.Warnings[0], "private-sentinel") {
		t.Fatal("partial failure was not safely distinguished from empty results")
	}
}

func TestXiaohongshuStatusesRedirectsAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		code             int
	}{
		{"ready", `{"success":true,"data":{"is_logged_in":true,"username":"private-sentinel"}}`, "ready", 200},
		{"logout", `{"success":true,"data":{"is_logged_in":false}}`, "logged_out", 200},
		{"auth", `private-sentinel`, "auth_failed", 401},
		{"missing status", `{"success":true,"data":{}}`, "unavailable", 200},
		{"invalid", `{"success":false,"error":"private-sentinel"}`, "unavailable", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/api/v1/login/status" {
					t.Error("wrong status endpoint")
				}
				w.WriteHeader(tc.code)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			c := NewXiaohongshuClient(config.Xiaohongshu{Enabled: true, BaseURL: server.URL})
			if got := c.Status(context.Background()); got != tc.want {
				t.Fatalf("status = %s", got)
			}
		})
	}
	for _, enabled := range []bool{false, true} {
		c := NewXiaohongshuClient(config.Xiaohongshu{Enabled: enabled, BaseURL: "https://remote.example:18060"})
		c.http.Transport = sourceTransport(func(*http.Request) (*http.Response, error) {
			t.Fatal("disabled or remote service was contacted")
			return nil, nil
		})
		if got := c.Status(context.Background()); got != "disabled" && got != "unavailable" {
			t.Fatal("unexpected unsupported connection state")
		}
	}
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected = true }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer origin.Close()
	c := NewXiaohongshuClient(config.Xiaohongshu{Enabled: true, BaseURL: origin.URL, AuthToken: "private-sentinel"})
	_, err := c.Search(context.Background(), "课程")
	if err == nil || redirected || strings.Contains(err.Error(), "private-sentinel") {
		t.Fatal("redirect followed or credential exposed")
	}
	entered := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
	}))
	defer slow.Close()
	c = NewXiaohongshuClient(config.Xiaohongshu{Enabled: true, BaseURL: slow.URL})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := c.Search(ctx, "课程"); done <- err }()
	<-entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation lost")
		}
	case <-time.After(time.Second):
		t.Fatal("source did not stop promptly")
	}
}

func TestSourceURLAllowlistAndTruncation(t *testing.T) {
	for _, link := range []string{"https://pd.qq.com/s/source", "https://www.xiaohongshu.com/explore/66abcdef1234567890abcdef"} {
		if !IsSourceURL(link) {
			t.Fatal("public post rejected")
		}
		for _, suffix := range []string{"?xsec_token=secret", "#fragment", "/../../login", "?", "%2fother"} {
			if IsSourceURL(link + suffix) {
				t.Fatal("noncanonical source accepted")
			}
		}
	}
	for _, link := range []string{"https://www.xiaohongshu.com.evil.test/explore/66abcdef1234567890abcdef", "http://pd.qq.com/s/source", "https://user:secret@pd.qq.com/s/source", "https://api.x.zanao.com/thread/info", "https://www.xiaohongshu.com/user/profile/66abcdef1234567890abcdef"} {
		if IsSourceURL(link) {
			t.Fatal("unsafe source URL accepted")
		}
	}
	p := Post{}
	for i := 0; i < 100; i++ {
		appendDiscussion(&p, strings.Repeat("长", 700), "", 600)
	}
	if len(p.Discussion) != 80 || !p.Partial || len([]rune(p.Discussion[0])) > 601 {
		t.Fatal("discussion not bounded")
	}
}
