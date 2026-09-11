package tools

// Read-only API adaptation of jeanhua/ZanaoMCP at 6b91407.
// Header protocol attribution and MIT license: docs/third-party/ZanaoMCP-LICENSE.
import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jaketmoon/hdu-station/internal/config"
)

const zanaoBaseURL = "https://api.x.zanao.com"

type ZanaoClient struct {
	cfg    config.Zanao
	http   *http.Client
	device string
}

func NewZanaoClient(cfg config.Zanao) *ZanaoClient {
	var random [20]byte
	_, _ = rand.Read(random[:])
	for i := range random {
		random[i] = '0' + random[i]%10
	}
	return &ZanaoClient{cfg: cfg, http: sourceHTTPClient(25*time.Second, false), device: string(random[:])}
}

func (c *ZanaoClient) request(ctx context.Context, path string, query, form url.Values, out any) error {
	if err := (config.Sources{Zanao: c.cfg}).Validate(); err != nil || !c.cfg.Enabled {
		return errSourceAuth
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	sign := md5.Sum([]byte(c.cfg.SchoolAlias + "_" + c.device + "_" + timestamp + "_1b6d2514354bc407afdd935f45521a8c"))
	headers := http.Header{}
	for name, value := range map[string]string{
		"X-Sc-Version": "3.4.4", "X-Sc-Nwt": "wifi", "X-Sc-Wf": "", "X-Sc-Nd": c.device,
		"X-Sc-Cloud": "0", "X-Sc-Platform": "windows", "X-Sc-Appid": "wx3921ddb0258ff14f",
		"X-Sc-Alias": c.cfg.SchoolAlias, "X-Sc-Od": c.cfg.Token, "X-Sc-Ah": hex.EncodeToString(sign[:]),
		"X-Sc-Td": timestamp, "xweb_xhr": "1", "Content-Type": "application/x-www-form-urlencoded", "Accept": "*/*",
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 MicroMessenger/7.0.20.1781(0x6700143B) NetType/WIFI MiniProgramEnv/Windows WindowsWechat/WMPF WindowsWechat(0x63090c33)XWEB/14185",
	} {
		headers.Set(name, value)
	}
	var envelope struct {
		Errno *int            `json:"errno"`
		Data  json.RawMessage `json:"data"`
	}
	address := zanaoBaseURL + path
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	err := sourceJSON(ctx, c.http, http.MethodPost, address, strings.NewReader(form.Encode()), headers, &envelope)
	if err != nil {
		return err
	}
	if envelope.Errno == nil || *envelope.Errno != 0 || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return errSourceResponse
	}
	if json.Unmarshal(envelope.Data, out) != nil {
		return errSourceResponse
	}
	return nil
}

func (c *ZanaoClient) Status(ctx context.Context) string {
	if !c.cfg.Enabled {
		return "disabled"
	}
	if c.cfg.Token == "" || c.cfg.SchoolAlias == "" {
		return "not_configured"
	}
	var data struct {
		School string `json:"school_name"`
	}
	err := c.request(ctx, "/user/info", url.Values{"from": {"mine"}}, nil, &data)
	if errors.Is(err, errSourceAuth) {
		return "logged_out"
	}
	if err != nil || data.School == "" {
		return "unavailable"
	}
	return "ready"
}

// Zanao returns either data: [] or data: {list: []}.
type zanaoList[T any] []T

func (list *zanaoList[T]) UnmarshalJSON(raw []byte) error {
	if len(raw) > 0 && raw[0] == '[' {
		return json.Unmarshal(raw, (*[]T)(list))
	}
	var wrapped struct {
		List *[]T `json:"list"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil || wrapped.List == nil {
		return errSourceResponse
	}
	*list = *wrapped.List
	return nil
}

type zanaoPost struct {
	ID       string      `json:"thread_id"`
	Title    string      `json:"title"`
	Content  string      `json:"content"`
	Date     string      `json:"post_time"`
	Comments json.Number `json:"c_count"`
}
type zanaoComment struct {
	Content string         `json:"content"`
	Replies []zanaoComment `json:"reply_list"`
}

func (c *ZanaoClient) Search(ctx context.Context, query string) (SearchResult, error) {
	result := SearchResult{Posts: []Post{}}
	// Current and historical search are separate upstream categories. Keep both
	// bounded to one page, retaining useful results if one category fails.
	seen := map[string]bool{}
	for _, category := range []string{"10", "20"} {
		var posts zanaoList[zanaoPost]
		params := url.Values{"wd": {query}, "cur_page": {"1"}, "cate_id": {category}}
		if category == "20" {
			params.Set("range", "1y")
		}
		if err := c.request(ctx, "/thread/v2/search", params, nil, &posts); err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			result.Warnings = append(result.Warnings, sourceError("赞哦", err).Error())
			continue
		}
		for i, raw := range posts {
			if i >= 20 {
				break
			}
			if !sourcePostID.MatchString(raw.ID) || seen[raw.ID] {
				continue
			}
			seen[raw.ID] = true
			count, _ := strconv.Atoi(raw.Comments.String())
			title := raw.Title
			if title == "" {
				title = trim(raw.Content, 80)
			}
			result.Posts = append(result.Posts, Post{Source: "zanao", Title: trim(title, 240), Date: trim(raw.Date, 40), Comments: count, feed: raw.ID})
		}
	}
	return result, nil
}

func (c *ZanaoClient) Read(ctx context.Context, p Post) (Post, []string, error) {
	var detail struct {
		Detail zanaoPost `json:"detail"`
	}
	if err := c.request(ctx, "/thread/info", nil, url.Values{"id": {p.feed}}, &detail); err != nil {
		return p, nil, sourceError("赞哦", err)
	}
	raw := detail.Detail
	if raw.ID != p.feed || strings.TrimSpace(raw.Content) == "" {
		return p, nil, errors.New("赞哦帖子正文无法读取")
	}
	p.Content = trim(raw.Content, 6500)
	p.Partial = p.Content != strings.TrimSpace(raw.Content)
	if raw.Title != "" {
		p.Title = trim(raw.Title, 240)
	}
	if raw.Date != "" {
		p.Date = trim(raw.Date, 40)
	}
	// No verified web permalink exists in the upstream contract.
	p.Locator = "赞哦帖子 " + p.feed + "（学校：" + c.cfg.SchoolAlias + "）"
	var comments zanaoList[zanaoComment]
	if err := c.request(ctx, "/comment/list", url.Values{"id": {p.feed}}, nil, &comments); err != nil {
		if ctx.Err() != nil {
			return p, nil, ctx.Err()
		}
		p.Partial = true
		return p, []string{sourceError("赞哦评论", err).Error()}, nil
	}
	for _, comment := range comments {
		appendDiscussion(&p, comment.Content, "", 600)
		for _, reply := range comment.Replies {
			appendDiscussion(&p, reply.Content, "回复：", 400)
		}
	}
	// The upstream comments API supplies no completeness/pagination contract.
	p.Partial = true
	return p, []string{"赞哦只读取当前返回的评论和回复预览，未确认已覆盖全部讨论"}, nil
}
