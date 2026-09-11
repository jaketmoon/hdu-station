package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Client struct {
	root string
	mu   sync.Mutex
	next time.Time
	run  func(context.Context, ...string) (json.RawMessage, error)
}

func NewClient(root string) *Client { c := &Client{root: root}; c.run = c.command; return c }
func TencentCLIPathAt(root string) string {
	p, err := tencentPackageFor(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return ""
	}
	name := "tencent-channel-cli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(root, "tools", "tencent-channel-cli", tencentCLIVersion, name)
	ok, err := validInstalledTencentCLI(path, filepath.Join(filepath.Dir(path), "install.json"), p.Integrity)
	if err != nil || !ok {
		return ""
	}
	return path
}
func (c *Client) Status(ctx context.Context) string {
	if _, err := tencentPackageFor(runtime.GOOS, runtime.GOARCH); err != nil {
		return "unsupported"
	}
	if TencentCLIPathAt(c.root) == "" {
		return "not_installed"
	}
	data, err := c.run(ctx, "login", "status")
	if err != nil {
		return "unavailable"
	}
	var s struct {
		Valid bool `json:"valid"`
	}
	if json.Unmarshal(data, &s) != nil {
		return "unavailable"
	}
	if s.Valid {
		return "ready"
	}
	return "logged_out"
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, errors.New("响应过大")
	}
	return b.Buffer.Write(p)
}

// Only fixed typed read operations reach this runner; there is no shell interpreter.
func (c *Client) command(ctx context.Context, args ...string) (json.RawMessage, error) {
	binary := TencentCLIPathAt(c.root)
	if binary == "" {
		return nil, errors.New("QQ 频道连接组件尚未安装，请在设置中安装")
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, append(args, "--json")...)
	var out, stderr limitedBuffer
	command.Stdout = &out
	command.Stderr = &stderr
	for _, name := range []string{"HOME", "USERPROFILE", "PATH", "TMPDIR", "TEMP", "LANG", "SSL_CERT_FILE", "SSL_CERT_DIR", "SystemRoot"} {
		if v, ok := os.LookupEnv(name); ok {
			command.Env = append(command.Env, name+"="+v)
		}
	}
	err := command.Run()
	if ctx.Err() != nil {
		return nil, errors.New("QQ 频道读取超时或已停止")
	}
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal(out.Bytes(), &envelope) != nil {
		return nil, errors.New("QQ 频道响应无法解析，请稍后重试")
	}
	if !envelope.Success || err != nil {
		raw := string(envelope.Error)
		switch {
		case strings.Contains(raw, "153") || strings.Contains(raw, "频率"):
			return nil, errors.New("rate_limited")
		case strings.Contains(raw, "8011") || strings.Contains(raw, "未登录"):
			return nil, errors.New("QQ 登录已失效，请重新登录频道 CLI")
		case strings.Contains(raw, "130000") || strings.Contains(raw, "20047"):
			return nil, errors.New("当前账号无法读取这个频道，请检查是否已加入")
		default:
			return nil, errors.New("QQ 频道暂时无法读取，请稍后重试")
		}
	}
	return envelope.Data, nil
}
func (c *Client) read(ctx context.Context, progress func(string), args ...string) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := time.Until(c.next); wait > 0 {
		if err := waitFor(ctx, wait); err != nil {
			return nil, err
		}
	}
	data, err := c.run(ctx, args...)
	c.next = time.Now().Add(600 * time.Millisecond)
	if err != nil && err.Error() == "rate_limited" {
		progress("频道暂时限流，稍等一分钟后继续…")
		if err := waitFor(ctx, 70*time.Second); err != nil {
			return nil, err
		}
		data, err = c.run(ctx, args...)
		if err != nil && err.Error() == "rate_limited" {
			return nil, errors.New("频道仍在限流，请稍后再试")
		}
	}
	return data, err
}
func waitFor(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Session) searchQQ(ctx context.Context, q string) (SearchResult, error) {
	result := SearchResult{Posts: []Post{}}
	for _, guild := range s.guilds {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		data, err := s.client.read(ctx, s.progress, "feed", "search-guild-feeds", "--guild-id", guild, "--query", q)
		if err != nil {
			result.Warnings = append(result.Warnings, err.Error())
			continue
		}
		var raw struct {
			Feeds []struct {
				ID       string `json:"feed_id"`
				Title    string `json:"title"`
				Date     string `json:"create_time"`
				Comments int    `json:"comment_count"`
			} `json:"guild_feeds"`
		}
		if json.Unmarshal(data, &raw) != nil {
			result.Warnings = append(result.Warnings, "部分搜索结果无法解析")
			continue
		}
		for i, f := range raw.Feeds {
			if i >= 20 {
				break
			}
			if f.ID == "" || len(f.ID) > 256 {
				continue
			}
			p := Post{Source: "qq", Title: trim(f.Title, 240), Date: trim(f.Date, 40), Comments: f.Comments, guild: guild, feed: f.ID}
			result.Posts = append(result.Posts, p)
		}
	}
	return result, nil
}
func (s *Session) readQQ(ctx context.Context, p Post) (Post, []string, error) {
	warnings := []string{}
	data, err := s.client.read(ctx, s.progress, "feed", "get-feed-detail", "--guild-id", p.guild, "--feed-id", p.feed)
	if err != nil {
		return p, nil, err
	}
	var raw struct {
		Feed struct {
			Title   string `json:"title"`
			Content string `json:"content"`
			Channel string `json:"channel_id"`
			URL     string `json:"share_url"`
			Date    string `json:"create_time"`
		} `json:"feed"`
	}
	if json.Unmarshal(data, &raw) != nil || raw.Feed.Content == "" {
		return p, nil, errors.New("部分帖子正文无法读取")
	}
	p.Content = trim(raw.Feed.Content, 6500)
	p.Partial = p.Content != raw.Feed.Content
	p.channel = raw.Feed.Channel
	p.URL = shareURL(raw.Feed.URL)
	if raw.Feed.Title != "" {
		p.Title = trim(raw.Feed.Title, 240)
	}
	if raw.Feed.Date != "" {
		p.Date = trim(raw.Feed.Date, 40)
	}
	p.Discussion = []string{}
	cursor := ""
	for page := 0; page < 2; page++ {
		args := []string{"feed", "get-feed-comments", "--guild-id", p.guild, "--feed-id", p.feed, "--count", "20", "--reply-list-num", "5"}
		if p.channel != "" {
			args = append(args, "--channel-id", p.channel)
		}
		if cursor != "" {
			args = append(args, "--attach-info", cursor)
		}
		data, err = s.client.read(ctx, s.progress, args...)
		if err != nil {
			p.Partial = true
			warnings = append(warnings, err.Error())
			break
		}
		var comments struct {
			Items []struct {
				Text    string          `json:"content_text"`
				Content json.RawMessage `json:"content"`
				More    bool            `json:"has_more_replies"`
				Replies []struct {
					Text    string          `json:"content_text"`
					Content json.RawMessage `json:"content"`
				} `json:"replies_preview"`
			} `json:"comments"`
			More   bool   `json:"has_more"`
			Cursor string `json:"attach_info"`
		}
		if json.Unmarshal(data, &comments) != nil {
			p.Partial = true
			break
		}
		for _, c := range comments.Items {
			if text := commentText(c.Text, c.Content); text != "" {
				p.Discussion = append(p.Discussion, trim(text, 600))
			}
			for _, r := range c.Replies {
				if text := commentText(r.Text, r.Content); text != "" {
					p.Discussion = append(p.Discussion, "回复："+trim(text, 400))
				}
			}
			if c.More {
				p.Partial = true
			}
		}
		if !comments.More {
			break
		}
		cursor = comments.Cursor
		if cursor == "" || page == 1 {
			p.Partial = true
			break
		}
	}
	if len(p.Discussion) > 80 {
		p.Discussion = p.Discussion[:80]
		p.Partial = true
	}
	return p, warnings, nil
}
func commentText(text string, raw json.RawMessage) string {
	if text != "" {
		return text
	}
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var c struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(raw, &c)
	return c.Text
}
func shareURL(raw string) string {
	if !IsSourceURL(raw) || !strings.HasPrefix(raw, "https://pd.qq.com/") {
		return ""
	}
	return raw
}
func trim(text string, limit int) string {
	r := []rune(strings.TrimSpace(text))
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return string(r)
}

var _ io.Writer = (*limitedBuffer)(nil)
