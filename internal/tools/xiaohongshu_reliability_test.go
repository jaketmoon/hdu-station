package tools

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jaketmoon/hdu-station/internal/config"
)

func TestXiaohongshuRetriesTransientReadsAndKeepsDiagnosticsPrivate(t *testing.T) {
	for _, failure := range []string{"timeout", "server_error"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			c := NewXiaohongshuClient(config.Xiaohongshu{Enabled: true, BaseURL: "http://127.0.0.1:18060", AuthToken: "credential-private-sentinel"}, root)
			calls := 0
			c.http.Transport = sourceTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				body, _ := io.ReadAll(r.Body)
				if !strings.Contains(string(body), "query-private-sentinel") || r.Header.Get("Authorization") == "" {
					t.Error("retry did not preserve the read request")
				}
				if calls == 1 && failure == "timeout" {
					return nil, &net.DNSError{Err: "credential-private-sentinel", IsTimeout: true}
				}
				code, payload := 200, `{"success":true,"data":{"feeds":[]}}`
				if calls == 1 {
					code, payload = 500, `{"details":"credential-private-sentinel"}`
				}
				return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(payload)), Header: http.Header{}}, nil
			})
			result, err := c.Search(context.Background(), "query-private-sentinel")
			if err != nil || calls != 2 || len(result.Posts) != 0 {
				t.Fatal("transient read did not recover exactly once")
			}
			file := filepath.Join(root, "logs", "xiaohongshu-requests.jsonl")
			data, err := os.ReadFile(file)
			if err != nil || !strings.Contains(string(data), failure) || !strings.Contains(string(data), `"outcome":"ok"`) || strings.Contains(string(data), "private-sentinel") {
				t.Fatal("diagnostic categories missing or private request data logged")
			}
			info, err := os.Stat(file)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("diagnostics are not private")
			}
		})
	}
}

func TestXiaohongshuFailureCategoriesAndRetryLimit(t *testing.T) {
	for _, tc := range []struct {
		name      string
		code      int
		wantCalls int
		message   string
	}{
		{"auth", 401, 1, "连接验证失败"},
		{"security challenge", 428, 1, "安全验证"},
		{"rate limit", 429, 1, "限流"},
		{"invalid request", 400, 1, "数据格式无效"},
		{"server failure", 500, 2, "服务处理请求失败"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewXiaohongshuClient(config.Xiaohongshu{Enabled: true, BaseURL: "http://127.0.0.1:18060"})
			calls := 0
			c.http.Transport = sourceTransport(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: tc.code, Body: io.NopCloser(strings.NewReader("credential-private-sentinel")), Header: http.Header{}}, nil
			})
			_, err := c.Search(context.Background(), "高数")
			if err == nil || calls != tc.wantCalls || !strings.Contains(err.Error(), tc.message) || strings.Contains(err.Error(), "private-sentinel") || strings.Contains(err.Error(), "登录") {
				t.Fatal("failure misclassified, retried incorrectly, or exposed upstream data")
			}
		})
	}
	if message := sourceError("小红书", errSourceTimeout).Error(); !strings.Contains(message, "超时") || strings.Contains(message, "登录") {
		t.Fatal("timeout incorrectly attributed to login")
	}
}

func TestXiaohongshuCancelDuringRetryWait(t *testing.T) {
	c := NewXiaohongshuClient(config.Xiaohongshu{Enabled: true, BaseURL: "http://127.0.0.1:18060"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	entered := make(chan struct{})
	c.http.Transport = sourceTransport(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			close(entered)
		}
		return nil, &net.DNSError{IsTimeout: true}
	})
	done := make(chan error, 1)
	go func() { _, err := c.Search(ctx, "高数"); done <- err }()
	<-entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatal("canceled search retried or lost cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation waited for retry delay")
	}
}
