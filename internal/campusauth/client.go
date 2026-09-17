// Package campusauth uses the official hduhelp-cli Device Authorization Grant.
// Contract: hduhelp/hduhelp-neo@ba988e2, cmd/hduhelp-cli/auth.go and
// docs/hduhelp-cli.md. Requests course/schedule reads and simulation plan and course favorite access.
package campusauth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

const CourseScope = "academic:course:read"
const ScheduleScope = "academic:schedule:read"
const SimulationReadScope = "academic:coursesimulation:read"
const SimulationWriteScope = "academic:coursesimulation:write"
const FavoriteReadScope = "academic:coursefavorite:read"
const FavoriteWriteScope = "academic:coursefavorite:write"
const requestedScope = CourseScope + " " + ScheduleScope + " " + SimulationReadScope + " " + SimulationWriteScope + " " + FavoriteReadScope + " " + FavoriteWriteScope

var (
	ErrLoginRequired = errors.New("校园授权无效或已过期，请在助手设置中重新授权")
	ErrUnavailable   = errors.New("校园授权服务暂时无法连接，请稍后重试")
	ErrStorage       = errors.New("无法保存校园授权，请检查应用数据目录后重试")
	ErrResponse      = errors.New("校园授权服务返回无效响应，请重新授权")
	ErrScope         = errors.New("缺少所需校园权限，请在助手设置的 HDU CLI 登录中重新授权")
)

type credentials struct {
	Version   int      `yaml:"version"`
	DeviceID  string   `yaml:"device_id"`
	Token     string   `yaml:"token,omitempty" json:"-"`
	Scopes    []string `yaml:"scopes,omitempty"`
	ExpiresAt int64    `yaml:"expires_at,omitempty"`     // Unix milliseconds; zero means no expiry.
	Managed   bool     `yaml:"web_authorized,omitempty"` // Only revoke PATs created by Station.
}

type Client struct {
	mu          sync.Mutex
	credentials credentials
	attempt     *attempt
	closed      bool
	http        *apiClient
	save        func(credentials) error
	wait        func(context.Context, time.Duration) bool
}

// A legacy manual PAT is used only until the first web login or local logout.
// Station never reads or modifies a shared hduhelp-cli configuration.
func New(root, legacyPAT string) (*Client, error) {
	if !validToken(legacyPAT) {
		return nil, ErrStorage
	}
	c := &Client{http: newAPIClient(), wait: waitForPoll}
	c.save = func(s credentials) error { return saveCredentials(root, s) }
	c.credentials = credentials{Version: 1, DeviceID: uuid.NewString(), Token: legacyPAT}
	path := filepath.Join(root, "campus-auth.yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, ErrStorage
	}
	var stored credentials
	if len(data) > 16<<10 || yaml.Unmarshal(data, &stored) != nil || stored.Version != 1 {
		return nil, errors.New("校园授权文件格式或版本不受支持")
	}
	if !validToken(stored.Token) || stored.DeviceID == "" || len(stored.DeviceID) > 64 || !validToken(stored.DeviceID) || stored.ExpiresAt < 0 {
		return nil, ErrStorage
	}
	if os.Chmod(path, 0600) != nil {
		return nil, ErrStorage
	}
	c.credentials = stored
	return c, nil
}

func validToken(s string) bool { return len(s) <= 4096 && !strings.ContainsAny(s, " \t\r\n\x00") }

func saveCredentials(root string, s credentials) error {
	if os.MkdirAll(root, 0700) != nil {
		return ErrStorage
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return ErrStorage
	}
	f, err := os.CreateTemp(root, ".campus-auth-*")
	if err != nil {
		return ErrStorage
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return ErrStorage
	}
	if f.Sync() != nil || f.Close() != nil {
		return ErrStorage
	}
	if os.Rename(f.Name(), filepath.Join(root, "campus-auth.yaml")) != nil {
		return ErrStorage
	}
	return nil
}

func (c *Client) Configured() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && c.credentials.Token != ""
}

// Status reports local login state only. No business API is used to probe a
// saved credential; future campus capabilities must validate their own access.
func (c *Client) Status() string {
	if c == nil {
		return "logged_out"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.credentials.Token == "" {
		return "logged_out"
	}
	if c.credentials.ExpiresAt != 0 && c.credentials.ExpiresAt <= time.Now().UnixMilli() {
		return "expired"
	}
	return "saved"
}

func (c *Client) AccessToken(ctx context.Context) (string, error) {
	if c == nil {
		return "", ErrLoginRequired
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if c.closed || c.credentials.Token == "" || (c.credentials.ExpiresAt != 0 && c.credentials.ExpiresAt <= time.Now().UnixMilli()) {
		return "", ErrLoginRequired
	}
	return c.credentials.Token, nil
}

// Permission metadata is safe for settings; credentials never cross Wails.
func (c *Client) HasScope(scope string) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, approved := range c.credentials.Scopes {
		if approved == scope {
			return true
		}
	}
	return false
}

func (c *Client) AccessTokenFor(ctx context.Context, scope string) (string, error) {
	token, err := c.AccessToken(ctx)
	if err != nil {
		return "", err
	}
	if c.HasScope(scope) {
		return token, nil
	}
	// Old manually imported PATs have unknown scopes. Preserve course reads;
	// personal schedule access requires an explicitly recorded grant.
	c.mu.Lock()
	legacyCourse := !c.credentials.Managed && len(c.credentials.Scopes) == 0 && scope == CourseScope
	c.mu.Unlock()
	if legacyCourse {
		return token, nil
	}
	return "", ErrScope
}

func (c *Client) Logout(ctx context.Context) (string, error) {
	c.cancel("")
	c.mu.Lock()
	old := c.credentials
	next := credentials{Version: 1, DeviceID: old.DeviceID}
	if c.save(next) != nil {
		c.mu.Unlock()
		return "", ErrStorage
	}
	c.credentials = next
	c.mu.Unlock()
	if old.Managed && old.Token != "" {
		if err := c.http.revoke(ctx, old.Token); err != nil {
			return "本机已退出；未能确认远端撤销，可在官方令牌管理页移除 HDU Station 授权。", nil
		}
	}
	return "已退出校园授权。", nil
}

func (c *Client) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.cancel("")
}

func Supported(goos string) bool { return goos == "darwin" || goos == "windows" || goos == "linux" }
