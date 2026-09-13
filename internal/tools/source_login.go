package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

// LoginChallenge is settings-only. Account tokens, device codes and cookie
// paths never cross the Wails boundary or enter an agent tool response.
type LoginChallenge struct {
	Kind      string `json:"kind,omitempty"`
	Status    string `json:"status"`
	Image     string `json:"image,omitempty"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
}

func loginImage(raw string) (string, error) {
	if len(raw) > 1<<20 {
		return "", errors.New("登录二维码无效，请重新连接")
	}
	encoded := raw
	if strings.HasPrefix(raw, "data:") {
		prefix, data, ok := strings.Cut(raw, ",")
		if !ok || (prefix != "data:image/png;base64" && prefix != "data:image/jpeg;base64") {
			return "", errors.New("登录二维码无效，请重新连接")
		}
		encoded = data
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("登录二维码无效，请重新连接")
	}
	info, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") || info.Width < 1 || info.Height < 1 || info.Width > 2048 || info.Height > 2048 {
		return "", errors.New("登录二维码无效，请重新连接")
	}
	return "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func (c *Client) BeginLogin(ctx context.Context) (LoginChallenge, error) {
	dir := filepath.Join(c.root, "tools", "qq-login")
	if os.MkdirAll(dir, 0700) != nil {
		return LoginChallenge{}, errors.New("无法准备登录二维码")
	}
	path := filepath.Join(dir, "qrcode.png")
	defer os.Remove(path)
	// The user explicitly chooses Reconnect, authorizing a replacement login.
	data, err := c.run(ctx, "login", "--yes", "--qrcode-path", path)
	if err != nil {
		return LoginChallenge{}, errors.New("无法获取 QQ 登录二维码，请稍后重新连接")
	}
	var result struct {
		Image   string `json:"qr_code"`
		Expires int    `json:"expires_in_s"`
	}
	if json.Unmarshal(data, &result) != nil {
		return LoginChallenge{}, errors.New("无法获取 QQ 登录二维码，请重新连接")
	}
	img, err := loginImage(result.Image)
	if err != nil {
		return LoginChallenge{}, err
	}
	if result.Expires <= 0 || result.Expires > 600 {
		return LoginChallenge{}, errors.New("QQ 登录二维码已失效，请重新连接")
	}
	return LoginChallenge{Status: "waiting", Image: img, ExpiresAt: time.Now().Add(time.Duration(result.Expires) * time.Second).UnixMilli()}, nil
}

func (c *Client) CompleteLogin(ctx context.Context) (string, error) {
	data, err := c.run(ctx, "login", "poll-token")
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", errors.New("QQ 登录未完成，请重新连接后扫码授权")
	}
	var result struct {
		Status       string `json:"status"`
		Connectivity string `json:"connectivity"`
	}
	if json.Unmarshal(data, &result) != nil || result.Status != "authorized" {
		return "", errors.New("QQ 登录未完成，请重新连接后扫码授权")
	}
	if result.Connectivity != "ok" {
		return "saved", nil
	}
	return "ready", nil
}

// The pinned QQ CLI stores the shared account in keyring service qq-cli,
// account token, with a dotenv fallback. Never remove unrelated keyring entries
// or rewrite the other variables in a shared dotenv file.
func (c *Client) ClearCredentials(ctx context.Context) error {
	if _, err := tencentPackageFor(runtime.GOOS, runtime.GOARCH); err != nil {
		return errors.New("当前平台暂不支持 QQ 连接")
	}
	data, err := c.run(ctx, "login", "status")
	if err != nil {
		return errors.New("无法检查 QQ 登录凭证，请稍后重试")
	}
	var status struct {
		Source string `json:"tokenSource"`
	}
	if json.Unmarshal(data, &status) != nil {
		return errors.New("无法检查 QQ 登录凭证，请稍后重试")
	}
	if status.Source == "mcporter" {
		return errors.New("QQ 凭证由外部工具管理，请在原工具中退出登录")
	}
	if err := keyring.Delete("qq-cli", "token"); err != nil && !errors.Is(err, keyring.ErrNotFound) && status.Source == "keychain" {
		return errors.New("无法清除 QQ 登录凭证，请稍后重试")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return errors.New("无法访问 QQ 登录凭证")
	}
	for _, dir := range []string{".qqcli", ".openclaw"} {
		if err := clearQQDotenv(filepath.Join(home, dir, ".env")); err != nil {
			return err
		}
	}
	if err := os.Remove(filepath.Join(home, ".qqcli", "device_auth_state.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("无法清除 QQ 待登录状态")
	}
	data, err = c.run(ctx, "login", "status")
	if err != nil {
		return errors.New("QQ 凭证已清除，暂时无法确认登录状态，请重新检查")
	}
	var remaining struct {
		Valid *bool `json:"valid"`
	}
	if json.Unmarshal(data, &remaining) != nil || remaining.Valid == nil {
		return errors.New("暂时无法确认 QQ 登录状态，请重新检查")
	}
	if *remaining.Valid {
		return errors.New("QQ 仍在使用外部工具提供的登录，请在原工具中退出登录")
	}
	return nil
}

func clearQQDotenv(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("无法清除 QQ 登录凭证")
	}
	lines := strings.Split(string(data), "\n")
	changed := false
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		name, _, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export ")), "=")
		if ok && strings.TrimSpace(name) == "QQ_AI_CONNECT_TOKEN" {
			changed = true
			continue
		}
		kept = append(kept, line)
	}
	if !changed {
		return nil
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".qq-logout-*")
	if err != nil {
		return errors.New("无法清除 QQ 登录凭证")
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.WriteString(strings.Join(kept, "\n")); err != nil {
		return errors.New("无法清除 QQ 登录凭证")
	}
	if err = f.Close(); err != nil {
		return errors.New("无法清除 QQ 登录凭证")
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return errors.New("无法清除 QQ 登录凭证")
	}
	return nil
}

func (c *XiaohongshuClient) BeginLogin(ctx context.Context) (LoginChallenge, error) {
	var result struct {
		LoggedIn *bool  `json:"is_logged_in"`
		Image    string `json:"img"`
		Timeout  string `json:"timeout"`
	}
	if err := c.request(ctx, "/api/v1/login/qrcode", nil, &result); err != nil {
		return LoginChallenge{}, sourceError("小红书", err)
	}
	if result.LoggedIn == nil {
		return LoginChallenge{}, sourceError("小红书", errSourceResponse)
	}
	if *result.LoggedIn {
		return LoginChallenge{Status: "ready"}, nil
	}
	img, err := loginImage(result.Image)
	if err != nil {
		return LoginChallenge{}, err
	}
	timeout, err := time.ParseDuration(result.Timeout)
	if err != nil || timeout <= 0 || timeout > 10*time.Minute {
		return LoginChallenge{}, errors.New("小红书登录二维码已失效，请重新连接")
	}
	return LoginChallenge{Status: "waiting", Image: img, ExpiresAt: time.Now().Add(timeout).UnixMilli()}, nil
}

func (c *XiaohongshuClient) ClearCredentials(ctx context.Context) error {
	var result struct{}
	if err := c.request(ctx, "/api/v1/login/cookies", nil, &result); err != nil {
		return sourceError("小红书", err)
	}
	return nil
}
