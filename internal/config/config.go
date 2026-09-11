package config

import (
	"errors"
	"gopkg.in/yaml.v3"
	"net/url"
	"os"
	"path/filepath"
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
	Version   int    `yaml:"version"`
	Model     Model  `yaml:"model"`
	CampusKey string `yaml:"campus_key,omitempty" json:"-"`
}

func Default() Config {
	return Config{Version: 1, Model: Model{BaseURL: "https://api.deepseek.com", Name: "deepseek-flash"}}
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
	return nil
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
	var c Config
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
