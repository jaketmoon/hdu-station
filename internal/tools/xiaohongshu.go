package tools

// Uses the documented HTTP API of xpzouying/xiaohongshu-mcp (aad2a3d).
// The local service owns its browser and cookies. The agent only has the three
// read endpoints; login endpoints are invoked exclusively from desktop settings.
import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jaketmoon/hdu-station/internal/config"
)

type XiaohongshuClient struct {
	cfg  config.Xiaohongshu
	http *http.Client
	root string
}

func NewXiaohongshuClient(cfg config.Xiaohongshu, roots ...string) *XiaohongshuClient {
	c := &XiaohongshuClient{cfg: cfg, http: sourceHTTPClient(75*time.Second, true)}
	if len(roots) > 0 {
		c.root = roots[0]
	}
	return c
}

func (c *XiaohongshuClient) request(ctx context.Context, path string, input any, out any) error {
	attempts := 1
	if path == "/api/v1/feeds/search" || path == "/api/v1/feeds/detail" {
		attempts = 2
	}
	for attempt := 1; ; attempt++ {
		start := time.Now()
		err := c.requestOnce(ctx, path, input, out)
		recordXiaohongshuRequest(c.root, path, attempt, start, err)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil || attempt == attempts ||
			(!errors.Is(err, errSourceTimeout) && !errors.Is(err, errSourceServer)) {
			return err
		}
		// These fixed endpoints only read. Reopening one browser request can
		// recover a transient timeout; auth failures and rate limits are final.
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *XiaohongshuClient) requestOnce(ctx context.Context, path string, input any, out any) error {
	if !c.cfg.Enabled || config.ValidateXiaohongshuURL(c.cfg.BaseURL) != nil {
		return errSourceUnavailable
	}
	method, body := http.MethodGet, ""
	if path == "/api/v1/login/cookies" {
		method = http.MethodDelete
	}
	headers := http.Header{}
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return errSourceResponse
		}
		method, body = http.MethodPost, string(data)
		headers.Set("Content-Type", "application/json")
	}
	if c.cfg.AuthToken != "" {
		headers.Set("Authorization", "Bearer "+c.cfg.AuthToken)
	}
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := sourceJSON(ctx, c.http, method, strings.TrimRight(c.cfg.BaseURL, "/")+path, strings.NewReader(body), headers, &envelope); err != nil {
		return err
	}
	if !envelope.Success || len(envelope.Data) == 0 || string(envelope.Data) == "null" || json.Unmarshal(envelope.Data, out) != nil {
		return errSourceResponse
	}
	return nil
}

func (c *XiaohongshuClient) Status(ctx context.Context) string {
	if !c.cfg.Enabled {
		return "disabled"
	}
	var data struct {
		LoggedIn *bool `json:"is_logged_in"`
	}
	err := c.request(ctx, "/api/v1/login/status", nil, &data)
	if errors.Is(err, errSourceAuth) {
		return "auth_failed"
	}
	if err != nil || data.LoggedIn == nil {
		return "unavailable"
	}
	if !*data.LoggedIn {
		return "logged_out"
	}
	return "ready"
}

func (c *XiaohongshuClient) Search(ctx context.Context, query string) (SearchResult, error) {
	result := SearchResult{Posts: []Post{}}
	// Unlike campus-scoped sources, Xiaohongshu searches across all schools.
	if !strings.Contains(query, "杭电") && !strings.Contains(query, "杭州电子科技大学") {
		query = "杭电 " + query
	}
	var data struct {
		Feeds *[]struct {
			ID    string `json:"id"`
			Token string `json:"xsecToken"`
			Type  string `json:"modelType"`
			Card  struct {
				Title       string `json:"displayTitle"`
				Interaction struct {
					Comments string `json:"commentCount"`
				} `json:"interactInfo"`
			} `json:"noteCard"`
		} `json:"feeds"`
	}
	if err := c.request(ctx, "/api/v1/feeds/search", map[string]string{"keyword": query}, &data); err != nil {
		return result, sourceError("小红书", err)
	}
	if data.Feeds == nil {
		return result, sourceError("小红书", errSourceResponse)
	}
	for _, raw := range *data.Feeds {
		if len(result.Posts) >= 20 {
			break
		}
		if (raw.Type != "" && raw.Type != "note") || !xiaohongshuPostID.MatchString(raw.ID) {
			continue
		}
		if raw.Token == "" || len(raw.Token) > 4096 || strings.ContainsAny(raw.Token, "\r\n\x00") {
			result.Warnings = append(result.Warnings, "部分小红书帖子缺少访问签名，无法读取，已跳过")
			continue
		}
		count, _ := strconv.Atoi(raw.Card.Interaction.Comments)
		result.Posts = append(result.Posts, Post{Source: "xiaohongshu", Title: trim(raw.Card.Title, 240), Comments: count, feed: raw.ID, xsecToken: raw.Token})
	}
	return result, nil
}

type xiaohongshuComment struct {
	Content    string               `json:"content"`
	ReplyCount string               `json:"subCommentCount"`
	Replies    []xiaohongshuComment `json:"subComments"`
}

func (c *XiaohongshuClient) Read(ctx context.Context, p Post) (Post, []string, error) {
	var data struct {
		Data struct {
			Note struct {
				ID          string            `json:"noteId"`
				Title       string            `json:"title"`
				Content     string            `json:"desc"`
				Time        int64             `json:"time"`
				Images      []json.RawMessage `json:"imageList"`
				Video       json.RawMessage   `json:"video"`
				Interaction struct {
					Comments string `json:"commentCount"`
				} `json:"interactInfo"`
			} `json:"note"`
			Comments *struct {
				List []xiaohongshuComment `json:"list"`
				More bool                 `json:"hasMore"`
			} `json:"comments"`
		} `json:"data"`
	}
	input := struct {
		ID    string `json:"feed_id"`
		Token string `json:"xsec_token"`
		All   bool   `json:"load_all_comments"`
	}{p.feed, p.xsecToken, false}
	if err := c.request(ctx, "/api/v1/feeds/detail", input, &data); err != nil {
		return p, nil, sourceError("小红书", err)
	}
	raw := data.Data.Note
	if raw.ID != p.feed || strings.TrimSpace(raw.Title+raw.Content) == "" {
		return p, nil, errors.New("小红书帖子正文无法读取")
	}
	p.Title, p.Content = trim(raw.Title, 240), trim(raw.Content, 6500)
	p.Partial = p.Content != strings.TrimSpace(raw.Content)
	p.URL = "https://www.xiaohongshu.com/explore/" + p.feed
	if raw.Time > 0 {
		p.Date = time.UnixMilli(raw.Time).UTC().Format("2006-01-02")
	}
	if count, err := strconv.Atoi(raw.Interaction.Comments); err == nil {
		p.Comments = count
	}
	warnings := []string{}
	if len(raw.Images) > 0 || (len(raw.Video) > 0 && string(raw.Video) != "null") {
		p.Partial = true
		warnings = append(warnings, "小红书只读取文字和评论，图片及音视频内容未读取")
	}
	comments := data.Data.Comments
	if comments == nil {
		p.Partial = true
		warnings = append(warnings, "小红书评论未能读取")
	} else {
		p.Partial = p.Partial || comments.More
		for _, comment := range comments.List {
			appendDiscussion(&p, comment.Content, "", 600)
			for _, reply := range comment.Replies {
				appendDiscussion(&p, reply.Content, "回复：", 400)
			}
			count, err := strconv.Atoi(comment.ReplyCount)
			if err != nil || count > len(comment.Replies) {
				p.Partial = true
			}
		}
		if p.Comments > len(p.Discussion) {
			p.Partial = true
		}
	}
	return p, warnings, nil
}

// The signed address is for the host's in-memory browser-opening map only.
// Tool JSON and SQLite contain the public, unsigned URL.
func (p Post) BrowserURL() string {
	if p.Source == "xiaohongshu" && p.xsecToken != "" && IsSourceURL(p.URL) {
		return p.URL + "?" + url.Values{"xsec_token": {p.xsecToken}, "xsec_source": {"pc_search"}}.Encode()
	}
	return p.URL
}
