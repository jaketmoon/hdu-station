package tools

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/jaketmoon/hdu-station/internal/config"
)

type Post struct {
	ID                              string   `json:"id"`
	Source                          string   `json:"source"`
	Title                           string   `json:"title"`
	Date                            string   `json:"date,omitempty"`
	Comments                        int      `json:"commentCount"`
	Content                         string   `json:"content,omitempty"`
	Discussion                      []string `json:"discussion,omitempty"`
	URL                             string   `json:"url,omitempty"`
	Locator                         string   `json:"locator,omitempty"`
	Partial                         bool     `json:"partial,omitempty"`
	guild, feed, channel, xsecToken string
}
type SearchInput struct {
	Query  string `json:"query" jsonschema:"description=简短的课程名、老师名或选课关键词"`
	Source string `json:"source,omitempty" jsonschema:"enum=all,enum=qq,enum=zanao,enum=xiaohongshu,description=可选来源；省略或 all 搜索全部已启用来源"`
}
type SearchResult struct {
	Posts    []Post   `json:"posts"`
	Warnings []string `json:"warnings,omitempty"`
}
type ReadInput struct {
	Posts []string `json:"posts" jsonschema:"description=搜索结果中的帖子 id，可混合不同来源，一次最多六个"`
}
type ReadResult struct {
	Posts    []Post   `json:"posts"`
	Warnings []string `json:"warnings,omitempty"`
}
type Session struct {
	client                 *Client
	guilds                 []string
	posts, reads           map[string]Post
	progress               func(string)
	sources                config.Sources
	zanao                  *ZanaoClient
	xiaohongshu            *XiaohongshuClient
	Calls, Searches, Reads int
}

func NewSession(c *Client, guilds []string, progress func(string), sources ...config.Sources) *Session {
	if progress == nil {
		progress = func(string) {}
	}
	var cfg config.Sources
	if len(sources) > 0 {
		cfg = sources[0]
	}
	root := ""
	if c != nil {
		root = c.root
	}
	return &Session{client: c, guilds: guilds, posts: map[string]Post{}, reads: map[string]Post{}, progress: progress, sources: cfg, zanao: NewZanaoClient(cfg.Zanao), xiaohongshu: NewXiaohongshuClient(cfg.Xiaohongshu, root)}
}

func (s *Session) SourceSummary() string {
	names := []string{}
	if !s.sources.QQ.Disabled {
		names = append(names, "QQ 频道（qq）")
	}
	if s.sources.Zanao.Enabled {
		names = append(names, "赞哦（zanao）")
	}
	if s.sources.Xiaohongshu.Enabled {
		names = append(names, "小红书（xiaohongshu）")
	}
	if len(names) == 0 {
		return "本轮没有启用搜索来源，请用户在设置中启用来源。不要声称已经查询社区讨论。"
	}
	return "本轮已启用的搜索来源：" + strings.Join(names, "、") + "。未启用的来源需要用户在设置中配置；工具失败不等于没有讨论。"
}

func (s *Session) Search(ctx context.Context, in SearchInput) (SearchResult, error) {
	result := SearchResult{Posts: []Post{}}
	q := strings.TrimSpace(in.Query)
	if len([]rune(q)) < 2 || len([]rune(q)) > 60 || strings.HasPrefix(q, "-") || strings.ContainsAny(q, "\n\r\x00") {
		result.Warnings = []string{"请使用 2–60 字的简短关键词"}
		return result, nil
	}
	if in.Source != "" && in.Source != "all" && in.Source != "qq" && in.Source != "zanao" && in.Source != "xiaohongshu" {
		result.Warnings = []string{"来源只能是 all、qq、zanao 或 xiaohongshu"}
		return result, nil
	}
	s.Calls++
	s.Searches++
	if s.Calls > 18 {
		result.Warnings = []string{"本轮读取已足够，请根据已有资料回答"}
		return result, nil
	}
	for _, source := range []struct {
		id, name string
		enabled  bool
		search   func(context.Context, string) (SearchResult, error)
	}{
		{"qq", "QQ 频道", !s.sources.QQ.Disabled, s.searchQQ},
		{"zanao", "赞哦", s.sources.Zanao.Enabled, s.zanao.Search},
		{"xiaohongshu", "小红书", s.sources.Xiaohongshu.Enabled, s.xiaohongshu.Search},
	} {
		if in.Source != "" && in.Source != "all" && in.Source != source.id {
			continue
		}
		if !source.enabled {
			if in.Source == source.id {
				result.Warnings = append(result.Warnings, source.name+"尚未启用，请在设置中配置")
			}
			continue
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		s.progress("正在" + source.name + "找「" + q + "」的同学讨论…")
		found, err := source.search(ctx, q)
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		}
		result.Warnings = append(result.Warnings, found.Warnings...)
		seen := map[string]bool{}
		for _, p := range found.Posts {
			for id, old := range s.posts {
				if old.Source == p.Source && old.feed == p.feed && old.guild == p.guild {
					p.ID = id
					break
				}
			}
			if p.ID == "" {
				p.ID = fmt.Sprintf("post-%d", len(s.posts)+1)
			}
			s.posts[p.ID] = p
			if !seen[p.ID] {
				result.Posts = append(result.Posts, p)
				seen[p.ID] = true
			}
		}
	}
	return result, nil
}

func (s *Session) Read(ctx context.Context, in ReadInput) (ReadResult, error) {
	result := ReadResult{Posts: []Post{}}
	if len(in.Posts) < 1 || len(in.Posts) > 6 {
		result.Warnings = []string{"一次请选择 1–6 个帖子"}
		return result, nil
	}
	s.Calls++
	if s.Calls > 18 {
		result.Warnings = []string{"本轮读取已足够，请根据已有资料回答"}
		return result, nil
	}
	for _, id := range in.Posts {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if p, ok := s.reads[id]; ok {
			result.Posts = append(result.Posts, p)
			continue
		}
		if s.Reads >= 12 {
			result.Warnings = append(result.Warnings, "本轮已读取 12 个帖子，请根据已有资料给出建议")
			break
		}
		p, ok := s.posts[id]
		if !ok {
			result.Warnings = append(result.Warnings, "帖子引用无效，请先搜索")
			continue
		}
		s.progress("正在读「" + trim(p.Title, 30) + "」和评论…")
		var err error
		var warnings []string
		switch p.Source {
		case "qq":
			p, warnings, err = s.readQQ(ctx, p)
		case "zanao":
			p, warnings, err = s.zanao.Read(ctx, p)
		case "xiaohongshu":
			p, warnings, err = s.xiaohongshu.Read(ctx, p)
		default:
			result.Warnings = append(result.Warnings, "帖子来源无效，请重新搜索")
			continue
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		result.Warnings = append(result.Warnings, warnings...)
		if err != nil {
			result.Warnings = append(result.Warnings, err.Error())
			continue
		}
		s.reads[id] = p
		s.Reads++
		result.Posts = append(result.Posts, p)
	}
	return result, nil
}

func (s *Session) Sources() []Post {
	posts := []Post{}
	for _, p := range s.reads {
		posts = append(posts, p)
	}
	sort.Slice(posts, func(i, j int) bool { return posts[i].ID < posts[j].ID })
	return posts
}

func appendDiscussion(p *Post, text, prefix string, limit int) {
	if strings.TrimSpace(text) == "" {
		return
	}
	if len(p.Discussion) >= 80 {
		p.Partial = true
		return
	}
	clipped := trim(text, limit)
	p.Partial = p.Partial || clipped != strings.TrimSpace(text)
	p.Discussion = append(p.Discussion, prefix+clipped)
}

var sourcePostID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var xiaohongshuPostID = regexp.MustCompile(`^[a-fA-F0-9]{24}$`)
var qqSourcePath = regexp.MustCompile(`^/s/[A-Za-z0-9]+$`)

func IsSourceURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" {
		return false
	}
	switch u.Host {
	case "pd.qq.com":
		return qqSourcePath.MatchString(u.Path)
	case "www.xiaohongshu.com":
		return strings.HasPrefix(u.Path, "/explore/") && xiaohongshuPostID.MatchString(strings.TrimPrefix(u.Path, "/explore/"))
	}
	return false
}
