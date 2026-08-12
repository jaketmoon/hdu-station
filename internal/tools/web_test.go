package tools

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jaketmoon/hdu-station/internal/config"
)

func TestWebSearchRejectsInvalidQueryAndMissingProviderKey(t *testing.T) {
	tool := NewWebSearchTool(configForProvider("brave"), nil)
	if _, err := tool.Search(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "between 2 and 240") {
		t.Fatalf("unexpected query error: %v", err)
	}
	if _, err := tool.Search(context.Background(), "course"); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("unexpected key error: %v", err)
	}
}

func TestWebFetchRejectsLocalAndCredentialURLs(t *testing.T) {
	tool := NewWebFetchTool(nil)
	for _, rawURL := range []string{
		"http://127.0.0.1:8080/health",
		"http://localhost/private",
		"https://student:secret@example.com/page",
	} {
		if _, _, err := tool.Fetch(context.Background(), rawURL); err == nil || !strings.Contains(err.Error(), "not allowed") && !strings.Contains(err.Error(), "without user credentials") {
			t.Fatalf("URL %q was not rejected: %v", rawURL, err)
		}
	}
}

func TestWebFetchRejectsRedirectsBeforeFollowingPrivateTarget(t *testing.T) {
	followed := false
	client := &http.Client{Transport: webRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/private" {
			followed = true
		}
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"http://127.0.0.1/private"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})}
	tool := NewWebFetchTool(client)
	_, _, err := tool.Fetch(context.Background(), "https://public.example/page")
	if err == nil || !strings.Contains(err.Error(), "web redirects are disabled") {
		t.Fatalf("redirect was not rejected: %v", err)
	}
	if followed {
		t.Fatal("web fetch followed a redirect to a private address")
	}
}

func TestWebToolBoundsInjectedClientTimeout(t *testing.T) {
	client := NewWebFetchTool(&http.Client{Timeout: 2 * time.Minute}).client
	if client.Timeout != webTimeout {
		t.Fatalf("web timeout = %s, want %s", client.Timeout, webTimeout)
	}
}

type webRoundTripFunc func(*http.Request) (*http.Response, error)

func (f webRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func configForProvider(provider string) config.WebSearchConfig {
	return config.WebSearchConfig{Provider: provider}
}
