package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jaketmoon/hdu-station/internal/config"
)

const (
	webTimeout     = 15 * time.Second
	maxWebBodySize = 1 << 20
)

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

type WebSearchTool struct {
	config config.WebSearchConfig
	client *http.Client
}

func NewWebSearchTool(cfg config.WebSearchConfig, client *http.Client) *WebSearchTool {
	return &WebSearchTool{config: cfg, client: newWebHTTPClient(client)}
}

func (tool *WebSearchTool) Definition() Definition {
	return Definition{
		Name:        "web_search",
		Description: "Search public web pages and return source titles, URLs, and short snippets.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","minLength":2,"maxLength":240}},"required":["query"],"additionalProperties":false}`),
	}
}

func (tool *WebSearchTool) ReadOnly() bool { return true }

func (tool *WebSearchTool) Call(ctx context.Context, arguments json.RawMessage) (Result, error) {
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode web_search arguments: %w", err)
	}
	results, err := tool.Search(ctx, input.Query)
	if err != nil {
		return Result{}, err
	}
	data, err := json.Marshal(results)
	if err != nil {
		return Result{}, fmt.Errorf("encode web_search result: %w", err)
	}
	return Result{Text: string(data)}, nil
}

func (tool *WebSearchTool) Search(ctx context.Context, query string) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 || len([]rune(query)) > 240 {
		return nil, errors.New("web search query must be between 2 and 240 characters")
	}
	switch tool.config.Provider {
	case "duckduckgo":
		return tool.searchDuckDuckGo(ctx, query)
	case "brave":
		return tool.searchBrave(ctx, query)
	case "tavily":
		return tool.searchTavily(ctx, query)
	default:
		return nil, fmt.Errorf("web search provider %q is not supported", tool.config.Provider)
	}
}

func (tool *WebSearchTool) searchDuckDuckGo(ctx context.Context, query string) ([]SearchResult, error) {
	endpoint := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	body, _, err := tool.get(ctx, endpoint, nil)
	if err != nil {
		return nil, err
	}
	linkPattern := regexp.MustCompile(`(?s)<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	snippetPattern := regexp.MustCompile(`(?s)<a[^>]*class="result__snippet"[^>]*>(.*?)</a>`)
	links := linkPattern.FindAllStringSubmatch(string(body), 10)
	snippets := snippetPattern.FindAllStringSubmatch(string(body), 10)
	results := make([]SearchResult, 0, len(links))
	for index, match := range links {
		if len(match) != 3 {
			continue
		}
		link, err := decodeSearchLink(match[1])
		if err != nil {
			continue
		}
		result := SearchResult{Title: stripHTML(match[2]), URL: link}
		if index < len(snippets) && len(snippets[index]) == 2 {
			result.Snippet = stripHTML(snippets[index][1])
		}
		results = append(results, result)
	}
	return results, nil
}

func (tool *WebSearchTool) searchBrave(ctx context.Context, query string) ([]SearchResult, error) {
	if strings.TrimSpace(tool.config.BraveKey) == "" {
		return nil, errors.New("Brave Search API key is not configured")
	}
	endpoint := "https://api.search.brave.com/res/v1/web/search?q=" + url.QueryEscape(query)
	body, _, err := tool.get(ctx, endpoint, map[string]string{"X-Subscription-Token": tool.config.BraveKey})
	if err != nil {
		return nil, err
	}
	var response struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode Brave Search response: %w", err)
	}
	results := make([]SearchResult, 0, len(response.Web.Results))
	for _, result := range response.Web.Results {
		results = append(results, SearchResult{Title: result.Title, URL: result.URL, Snippet: result.Description})
	}
	return results, nil
}

func (tool *WebSearchTool) searchTavily(ctx context.Context, query string) ([]SearchResult, error) {
	if strings.TrimSpace(tool.config.TavilyKey) == "" {
		return nil, errors.New("Tavily Search API key is not configured")
	}
	requestBody := struct {
		APIKey      string `json:"api_key"`
		Query       string `json:"query"`
		SearchDepth string `json:"search_depth"`
		MaxResults  int    `json:"max_results"`
	}{APIKey: tool.config.TavilyKey, Query: query, SearchDepth: "basic", MaxResults: 10}
	data, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("encode Tavily request: %w", err)
	}
	body, _, err := tool.post(ctx, "https://api.tavily.com/search", nil, data)
	if err != nil {
		return nil, err
	}
	var response struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode Tavily response: %w", err)
	}
	results := make([]SearchResult, 0, len(response.Results))
	for _, result := range response.Results {
		results = append(results, SearchResult{Title: result.Title, URL: result.URL, Snippet: result.Content})
	}
	return results, nil
}

type WebFetchTool struct {
	client *http.Client
}

func NewWebFetchTool(client *http.Client) *WebFetchTool {
	return &WebFetchTool{client: newWebHTTPClient(client)}
}

// newWebHTTPClient keeps the caller's transport and jar (which makes the
// client easy to test), but applies the security policy at the web-tool
// boundary. A caller must not be able to accidentally turn a public fetch
// into an unbounded request or let a response redirect to a private host.
func newWebHTTPClient(base *http.Client) *http.Client {
	client := &http.Client{}
	if base != nil {
		copy := *base
		client = &copy
	}
	if client.Timeout <= 0 || client.Timeout > webTimeout {
		client.Timeout = webTimeout
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return errors.New("web redirects are disabled")
	}
	return client
}

func (tool *WebFetchTool) Definition() Definition {
	return Definition{
		Name:        "web_fetch",
		Description: "Fetch a public HTTP(S) page for source verification.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","format":"uri","maxLength":2048}},"required":["url"],"additionalProperties":false}`),
	}
}

func (tool *WebFetchTool) ReadOnly() bool { return true }

func (tool *WebFetchTool) Call(ctx context.Context, arguments json.RawMessage) (Result, error) {
	var input struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode web_fetch arguments: %w", err)
	}
	body, contentType, err := tool.Fetch(ctx, input.URL)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: fmt.Sprintf("content-type: %s\n\n%s", contentType, body)}, nil
}

func (tool *WebFetchTool) Fetch(ctx context.Context, rawURL string) (string, string, error) {
	parsed, err := validatePublicURL(rawURL)
	if err != nil {
		return "", "", err
	}
	body, contentType, err := tool.get(ctx, parsed.String(), nil)
	if err != nil {
		return "", "", err
	}
	return string(body), contentType, nil
}

func (tool *WebSearchTool) get(ctx context.Context, endpoint string, headers map[string]string) ([]byte, string, error) {
	return request(ctx, tool.client, http.MethodGet, endpoint, headers, nil)
}

func (tool *WebSearchTool) post(ctx context.Context, endpoint string, headers map[string]string, body []byte) ([]byte, string, error) {
	return request(ctx, tool.client, http.MethodPost, endpoint, headers, body)
}

func (tool *WebFetchTool) get(ctx context.Context, endpoint string, headers map[string]string) ([]byte, string, error) {
	return request(ctx, tool.client, http.MethodGet, endpoint, headers, nil)
}

func request(ctx context.Context, client *http.Client, method, endpoint string, headers map[string]string, body []byte) ([]byte, string, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, "", fmt.Errorf("create web request: %w", err)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("call web provider: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("web provider returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxWebBodySize+1))
	if err != nil {
		return nil, "", fmt.Errorf("read web response: %w", err)
	}
	if len(data) > maxWebBodySize {
		return nil, "", errors.New("web response exceeds the 1 MiB limit")
	}
	return data, response.Header.Get("Content-Type"), nil
}

func validatePublicURL(rawURL string) (*url.URL, error) {
	if len(rawURL) > 2048 {
		return nil, errors.New("web URL exceeds the 2048 character limit")
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("web URL must be an HTTP(S) URL without user credentials")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return nil, errors.New("private and local web addresses are not allowed")
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()) {
		return nil, errors.New("private and local web addresses are not allowed")
	}
	return parsed, nil
}

func decodeSearchLink(raw string) (string, error) {
	raw = html.UnescapeString(raw)
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if redirect := parsed.Query().Get("uddg"); redirect != "" {
		return redirect, nil
	}
	return parsed.String(), nil
}

func stripHTML(value string) string {
	tagPattern := regexp.MustCompile(`(?s)<[^>]+>`)
	value = tagPattern.ReplaceAllString(value, " ")
	return strings.Join(strings.Fields(html.UnescapeString(value)), " ")
}
