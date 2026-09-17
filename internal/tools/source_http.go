package tools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

var errSourceConflict = errors.New("来源版本冲突")

var errSourceAuth = errors.New("来源认证失败")
var errSourceResponse = errors.New("来源响应无效")
var errSourceUnavailable = errors.New("来源暂时无法连接")
var errSourceRateLimit = errors.New("来源请求过于频繁")
var errSourceTimeout = errors.New("来源请求超时")
var errSourceServer = errors.New("来源服务处理失败")
var errSourceVerification = errors.New("小红书需要安全验证，请在设置中点击安全验证并扫码，本轮已跳过小红书")

func sourceHTTPClient(timeout time.Duration, local bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if local {
		transport.Proxy = nil
	}
	return &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

// Never expose upstream response bodies or transport errors: both may contain
// account credentials, signed URLs, or browser diagnostics.
func sourceJSON(ctx context.Context, client *http.Client, method, address string, body io.Reader, headers http.Header, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, address, body)
	if err != nil {
		return errSourceResponse
	}
	req.Header = headers
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			return errSourceTimeout
		}
		return errSourceUnavailable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusConflict:
		return errSourceConflict
	case resp.StatusCode == http.StatusPreconditionRequired:
		return errSourceVerification
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return errSourceAuth
	case resp.StatusCode == 429:
		return errSourceRateLimit
	case resp.StatusCode >= 500:
		return errSourceServer
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return errSourceResponse
	}
	const limit = 2 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return errSourceTimeout
	}
	if err != nil || len(data) > limit || json.Unmarshal(data, out) != nil {
		return errSourceResponse
	}
	return nil
}

func sourceError(name string, err error) error {
	if errors.Is(err, errSourceVerification) {
		return errSourceVerification
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, errSourceRateLimit) {
		return errors.New(name + "暂时限流，请稍后重试")
	}
	if errors.Is(err, errSourceTimeout) {
		return errors.New(name + "请求超时，请稍后重试")
	}
	if errors.Is(err, errSourceAuth) {
		if name == "小红书" {
			return errors.New("小红书连接验证失败，请检查本机连接组件")
		}
		return errors.New(name + "认证失败，请在设置中检查凭证")
	}
	if errors.Is(err, errSourceServer) {
		return errors.New(name + "服务处理请求失败，请稍后重试")
	}
	if errors.Is(err, errSourceResponse) {
		return errors.New(name + "返回的数据格式无效，请检查服务版本")
	}
	if name == "小红书" {
		return errors.New("小红书服务暂时无法连接，请检查本机服务是否运行")
	}
	return errors.New(name + "暂时无法连接，请稍后重试")
}
