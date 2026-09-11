package config

import (
	"errors"
	"gopkg.in/yaml.v3"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const AppDirectory = "HDU Station Course"

var OwnedEntries = []string{"config.yaml", "station.db", "station.db-wal", "station.db-shm", "tools", "logs"}

type Model struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key" json:"-"`
	Name    string `yaml:"name"`
}
type Config struct {
	Version   int     `yaml:"version"`
	Model     Model   `yaml:"model"`
	CampusKey string  `yaml:"campus_key,omitempty" json:"-"`
	Sources   Sources `yaml:"sources,omitempty"`
}

type Sources struct {
	QQ          QQ          `yaml:"qq"`
	Zanao       Zanao       `yaml:"zanao"`
	Xiaohongshu Xiaohongshu `yaml:"xiaohongshu"`
}

// Disabled keeps QQ enabled for existing configurations and zero-value callers.
type QQ struct {
	Disabled bool `yaml:"disabled"`
}
type Zanao struct {
	Enabled     bool   `yaml:"enabled"`
	SchoolAlias string `yaml:"school_alias"`
	Token       string `yaml:"token,omitempty" json:"-"`
}
type Xiaohongshu struct {
	Enabled   bool   `yaml:"enabled"`
	BaseURL   string `yaml:"base_url"`
	AuthToken string `yaml:"auth_token,omitempty" json:"-"`
}

const DefaultXiaohongshuURL = "http://127.0.0.1:18060"

var schoolAlias = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// Only connect to a user-run local service. No remote Station backend or
// executable path is accepted through this configuration.
func ValidateXiaohongshuURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("小红书需填写本机服务根地址，不含 /mcp、凭证或查询参数")
	}
	if u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1" {
		return errors.New("小红书服务只支持 127.0.0.1、localhost 或 [::1] 的本机地址")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return errors.New("小红书服务地址需包含有效端口，默认是 18060")
	}
	return nil
}

func (s Sources) Validate() error {
	for _, token := range []string{s.Zanao.Token, s.Xiaohongshu.AuthToken} {
		if len(token) > 4096 || strings.ContainsAny(token, "\r\n\x00") {
			return errors.New("搜索来源的 Token 格式不正确")
		}
	}
	if s.Zanao.SchoolAlias != "" && !schoolAlias.MatchString(s.Zanao.SchoolAlias) {
		return errors.New("赞哦学校别名只能包含字母、数字、下划线或连字符")
	}
	if s.Zanao.Enabled && (s.Zanao.Token == "" || s.Zanao.SchoolAlias == "") {
		return errors.New("启用赞哦前请填写 Token 和学校别名")
	}
	if s.Xiaohongshu.BaseURL != "" || s.Xiaohongshu.Enabled {
		return ValidateXiaohongshuURL(s.Xiaohongshu.BaseURL)
	}
	return nil
}

func Default() Config {
	return Config{Version: 1, Model: Model{BaseURL: "https://api.deepseek.com", Name: "deepseek-flash"}, Sources: Sources{Xiaohongshu: Xiaohongshu{BaseURL: DefaultXiaohongshuURL}}}
}
func Root() (string, error) {
	if root := os.Getenv("HDU_STATION_DATA_ROOT"); root != "" {
		if !filepath.IsAbs(root) || filepath.Clean(root) == string(filepath.Separator) {
			return "", errors.New("应用数据目录必须是独立的绝对路径")
		}
		return filepath.Clean(root), nil
	}
	base, err := os.UserConfigDir()
	return filepath.Join(base, AppDirectory), err
}
func (c Config) Validate() error {
	if c.Version != 1 {
		return errors.New("配置版本不受支持，请使用对应版本的应用")
	}
	u, err := url.Parse(c.Model.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("模型地址需为 HTTPS 地址，不含凭证或查询参数")
	}
	if c.Model.Name != "deepseek-flash" && c.Model.Name != "deepseek-v4.1-flash" {
		return errors.New("请使用 DeepSeek V4.1 Flash 的模型名称")
	}
	if len(c.Model.APIKey) > 4096 || strings.ContainsAny(c.Model.APIKey, "\r\n") {
		return errors.New("API Key 格式不正确")
	}
	return c.Sources.Validate()
}
func Load(root string) (Config, error) {
	path := filepath.Join(root, "config.yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, errors.New("无法读取本机配置")
	}
	c := Default()
	c.Version = 0 // Defaults for additive fields do not make a missing version valid.
	if yaml.Unmarshal(data, &c) != nil {
		return Config{}, errors.New("本机配置格式不正确")
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		return Config{}, errors.New("无法保护配置文件权限")
	}
	return c, nil
}
func Save(root string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return errors.New("无法创建应用数据目录")
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return errors.New("无法保存配置")
	}
	f, err := os.CreateTemp(root, ".config-*")
	if err != nil {
		return errors.New("无法保存配置")
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return errors.New("无法保存配置")
	}
	if err = f.Sync(); err != nil {
		return errors.New("无法保存配置")
	}
	if err = f.Close(); err != nil {
		return errors.New("无法保存配置")
	}
	if err = os.Rename(f.Name(), filepath.Join(root, "config.yaml")); err != nil {
		return errors.New("无法保存配置")
	}
	return nil
}
