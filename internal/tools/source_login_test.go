package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/zalando/go-keyring"
)

const testLoginPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jO1sAAAAASUVORK5CYII="

func TestLoginImageRejectsRemoteActiveAndOversizedContent(t *testing.T) {
	for _, raw := range []string{"https://example.com/qr.png", "data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=", "not-an-image", strings.Repeat("a", (1<<20)+1)} {
		if _, err := loginImage(raw); err == nil {
			t.Fatal("invalid login image accepted")
		}
	}
	for _, raw := range []string{testLoginPNG, "data:image/png;base64," + testLoginPNG} {
		if img, err := loginImage(raw); err != nil || !strings.HasPrefix(img, "data:image/png;base64,") {
			t.Fatal("login image rejected")
		}
	}
}
func TestQQLoginUsesFixedCommandsAndKeepsCredentialsPrivate(t *testing.T) {
	c := &Client{root: t.TempDir()}
	c.run = func(_ context.Context, args ...string) (json.RawMessage, error) {
		if len(args) != 4 || args[0] != "login" || args[1] != "--yes" || args[2] != "--qrcode-path" || !strings.HasPrefix(args[3], c.root) {
			t.Fatal("unexpected login command")
		}
		return json.RawMessage(`{"qr_code":"` + testLoginPNG + `","expires_in_s":240,"device_code":"device-private-sentinel","verification_uri":"https://example.com/private"}`), nil
	}
	challenge, err := c.BeginLogin(context.Background())
	if err != nil || challenge.Status != "waiting" || challenge.Image == "" {
		t.Fatal("login challenge missing")
	}
	data, _ := json.Marshal(challenge)
	if strings.Contains(string(data), "private") {
		t.Fatal("login internals crossed settings boundary")
	}
	c.run = func(_ context.Context, args ...string) (json.RawMessage, error) {
		if strings.Join(args, " ") != "login poll-token" {
			t.Fatal("login status cannot redeem a device token")
		}
		return json.RawMessage(`{"status":"authorized","connectivity":"ok","token":"token-private-sentinel"}`), nil
	}
	if status, err := c.CompleteLogin(context.Background()); err != nil || status != "ready" {
		t.Fatal("authorized login not completed")
	}
	c.run = func(_ context.Context, _ ...string) (json.RawMessage, error) {
		return json.RawMessage(`{"status":"authorized","connectivity":"unavailable"}`), nil
	}
	if status, err := c.CompleteLogin(context.Background()); err != nil || status != "saved" {
		t.Fatal("saved credentials treated as failed login")
	}
	c.run = func(_ context.Context, _ ...string) (json.RawMessage, error) {
		return nil, errors.New("token-private-sentinel")
	}
	if _, err := c.BeginLogin(context.Background()); err == nil || strings.Contains(err.Error(), "private-sentinel") {
		t.Fatal("upstream login error exposed")
	}
}
func TestQQClearOnlyRemovesItsCredentialAndPreservesSharedData(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if err := keyring.Set("qq-cli", "token", "private-sentinel"); err != nil {
		t.Fatal(err)
	}
	if err := keyring.Set("qq-cli", "device_id", "keep-device"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".qqcli", ".env")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# existing comment\nOTHER_TOKEN=keep-this\nexport QQ_AI_CONNECT_TOKEN='private-sentinel'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c := &Client{run: func(context.Context, ...string) (json.RawMessage, error) {
		if _, err := keyring.Get("qq-cli", "token"); errors.Is(err, keyring.ErrNotFound) {
			return json.RawMessage(`{"valid":false}`), nil
		}
		return json.RawMessage(`{"valid":true,"tokenSource":"keychain"}`), nil
	}}
	if err := c.ClearCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := keyring.Get("qq-cli", "token"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatal("QQ token not cleared")
	}
	if v, _ := keyring.Get("qq-cli", "device_id"); v != "keep-device" {
		t.Fatal("unrelated credential changed")
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "private-sentinel") || !strings.Contains(string(data), "OTHER_TOKEN=keep-this") || !strings.Contains(string(data), "# existing comment") {
		t.Fatal("dotenv clearing changed unrelated data")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("dotenv not private")
	}
}
func TestXiaohongshuSettingsLoginEndpointsAndCredentialIsolation(t *testing.T) {
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer service-private-sentinel" {
			t.Error("lost service authentication")
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/login/cookies":
			fmt.Fprint(w, `{"success":true,"data":{"cookie_path":"private-sentinel"}}`)
		case "/api/v1/login/qrcode":
			fmt.Fprint(w, `{"success":true,"data":{"is_logged_in":false,"timeout":"4m0s","img":"data:image/png;base64,`+testLoginPNG+`"}}`)
		default:
			t.Error("unexpected login endpoint")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c := NewXiaohongshuClient(config.Xiaohongshu{Enabled: true, BaseURL: server.URL, AuthToken: "service-private-sentinel"}, t.TempDir())
	if err := c.ClearCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}
	challenge, err := c.BeginLogin(context.Background())
	if err != nil || challenge.Status != "waiting" || challenge.Image == "" {
		t.Fatal("XHS login challenge missing")
	}
	if strings.Join(calls, ",") != "DELETE /api/v1/login/cookies,GET /api/v1/login/qrcode" {
		t.Fatal("unexpected login operations")
	}
	data, _ := json.Marshal(challenge)
	if strings.Contains(string(data), "private-sentinel") {
		t.Fatal("account data exposed")
	}
}
func TestDisabledQQNeverSearchesAndIsNotAdvertised(t *testing.T) {
	c := &Client{run: func(context.Context, ...string) (json.RawMessage, error) {
		t.Fatal("disabled source executed")
		return nil, nil
	}}
	s := NewSession(c, []string{"test"}, nil, config.Sources{QQ: config.QQ{Disabled: true}})
	result, err := s.Search(context.Background(), SearchInput{Source: "qq", Query: "通识选修"})
	if err != nil || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "尚未启用") {
		t.Fatal("disabled source was not reported")
	}
	if strings.Contains(s.SourceSummary(), "QQ 频道（qq）") {
		t.Fatal("disabled source advertised to model")
	}
}
