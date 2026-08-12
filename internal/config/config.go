package config

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

const CurrentVersion = 1

type Config struct {
	Version   int             `yaml:"version" json:"version"`
	Models    ModelsConfig    `yaml:"models" json:"models"`
	Campus    CampusConfig    `yaml:"campus" json:"campus"`
	WebSearch WebSearchConfig `yaml:"web_search" json:"web_search"`
	Sandbox   SandboxConfig   `yaml:"sandbox" json:"sandbox"`
}

type ModelsConfig struct {
	Default   string                    `yaml:"default" json:"default"`
	Providers map[string]ProviderConfig `yaml:"providers" json:"providers"`
}

type ProviderConfig struct {
	Type     string `yaml:"type" json:"type"`
	Protocol string `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	BaseURL  string `yaml:"base_url" json:"base_url"`
	APIKey   string `yaml:"api_key" json:"api_key"`
	Model    string `yaml:"model" json:"model"`
}

type CampusConfig struct {
	Key string `yaml:"key" json:"key"`
}

// CampusAuthStatus is deliberately derived from local configuration. It does
// not probe Neo or expose the credential itself. The current Neo contract only
// permits the RFC 8628 device flow for hduhelp-cli; Station must therefore not
// pretend that it has a first-party device client until the server registers
// one explicitly.
type CampusAuthStatus struct {
	State                    string `json:"state"`
	Configured               bool   `json:"configured"`
	Method                   string `json:"method"`
	DeviceAuthorization      string `json:"deviceAuthorization"`
	ServerClientRegistration bool   `json:"serverClientRegistrationRequired"`
	Notice                   string `json:"notice"`
}

const (
	CampusAuthNotConfigured = "not_configured"
	CampusAuthPATConfigured = "pat_configured_unverified"
	CampusAuthPATVerified   = "pat_verified"
	CampusAuthPATRejected   = "pat_rejected"
	CampusAuthScopeMissing  = "scope_missing"
	CampusAuthUnavailable   = "unavailable"
)

type WebSearchConfig struct {
	Provider  string `yaml:"provider" json:"provider"`
	BraveKey  string `yaml:"brave_api_key,omitempty" json:"brave_api_key,omitempty"`
	TavilyKey string `yaml:"tavily_api_key,omitempty" json:"tavily_api_key,omitempty"`
}

type SandboxConfig struct {
	ImageURL       string `yaml:"image_url,omitempty" json:"image_url,omitempty"`
	ImageSHA256    string `yaml:"image_sha256,omitempty" json:"image_sha256,omitempty"`
	ImageSignature string `yaml:"image_signature,omitempty" json:"image_signature,omitempty"`
	ImagePublicKey string `yaml:"image_public_key,omitempty" json:"image_public_key,omitempty"`
}

func Default() Config {
	return Config{
		Version: CurrentVersion,
		Models: ModelsConfig{
			Providers: map[string]ProviderConfig{
				"openai": {
					Type:     "openai",
					Protocol: "responses",
					BaseURL:  "https://api.openai.com/v1",
				},
				"anthropic": {
					Type:     "anthropic",
					Protocol: "messages",
					BaseURL:  "https://api.anthropic.com",
				},
			},
		},
		WebSearch: WebSearchConfig{Provider: "duckduckgo"},
	}
}

func FromEnvironment(lookup func(string) string) Config {
	cfg := Default()
	openAI := cfg.Models.Providers["openai"]
	openAI.APIKey = lookup("HDU_STATION_OPENAI_API_KEY")
	openAI.Model = lookup("HDU_STATION_OPENAI_MODEL")
	if value := lookup("HDU_STATION_OPENAI_BASE_URL"); value != "" {
		openAI.BaseURL = value
	}
	if value := lookup("HDU_STATION_OPENAI_PROTOCOL"); value != "" {
		openAI.Protocol = value
	}
	cfg.Models.Providers["openai"] = openAI

	anthropic := cfg.Models.Providers["anthropic"]
	anthropic.APIKey = lookup("HDU_STATION_ANTHROPIC_API_KEY")
	anthropic.Model = lookup("HDU_STATION_ANTHROPIC_MODEL")
	if value := lookup("HDU_STATION_ANTHROPIC_BASE_URL"); value != "" {
		anthropic.BaseURL = value
	}
	cfg.Models.Providers["anthropic"] = anthropic

	cfg.Campus.Key = lookup("HDU_STATION_CAMPUS_KEY")
	if value := lookup("HDU_STATION_WEB_SEARCH_PROVIDER"); value != "" {
		cfg.WebSearch.Provider = value
	}
	cfg.WebSearch.BraveKey = lookup("HDU_STATION_BRAVE_SEARCH_API_KEY")
	cfg.WebSearch.TavilyKey = lookup("HDU_STATION_TAVILY_API_KEY")
	cfg.Sandbox.ImageURL = lookup("HDU_STATION_SANDBOX_IMAGE_URL")
	cfg.Sandbox.ImageSHA256 = lookup("HDU_STATION_SANDBOX_IMAGE_SHA256")
	cfg.Sandbox.ImageSignature = lookup("HDU_STATION_SANDBOX_IMAGE_SIGNATURE")
	cfg.Sandbox.ImagePublicKey = lookup("HDU_STATION_SANDBOX_IMAGE_PUBLIC_KEY")

	if openAI.APIKey != "" && openAI.Model != "" {
		cfg.Models.Default = "openai"
	} else if anthropic.APIKey != "" && anthropic.Model != "" {
		cfg.Models.Default = "anthropic"
	}
	return cfg
}

func (cfg Config) Validate() error {
	if cfg.Version != CurrentVersion {
		return fmt.Errorf("version must be %d", CurrentVersion)
	}
	if cfg.Models.Providers == nil {
		return fmt.Errorf("models.providers is required")
	}
	if cfg.Models.Default != "" {
		if _, ok := cfg.Models.Providers[cfg.Models.Default]; !ok {
			return fmt.Errorf("models.default %q does not name a configured provider", cfg.Models.Default)
		}
	}

	providerNames := make([]string, 0, len(cfg.Models.Providers))
	for name := range cfg.Models.Providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)
	for _, name := range providerNames {
		provider := cfg.Models.Providers[name]
		if err := validateProvider(name, provider); err != nil {
			return err
		}
	}

	switch cfg.WebSearch.Provider {
	case "duckduckgo":
	case "brave":
		if strings.TrimSpace(cfg.WebSearch.BraveKey) == "" {
			return fmt.Errorf("web_search.brave_api_key is required for brave")
		}
	case "tavily":
		if strings.TrimSpace(cfg.WebSearch.TavilyKey) == "" {
			return fmt.Errorf("web_search.tavily_api_key is required for tavily")
		}
	default:
		return fmt.Errorf("web_search.provider must be duckduckgo, brave, or tavily")
	}
	if err := validateSandbox(cfg.Sandbox); err != nil {
		return err
	}
	return nil
}

func validateSandbox(sandbox SandboxConfig) error {
	imageURL := strings.TrimSpace(sandbox.ImageURL)
	imageSHA256 := strings.TrimSpace(sandbox.ImageSHA256)
	signature := strings.TrimSpace(sandbox.ImageSignature)
	publicKey := strings.TrimSpace(sandbox.ImagePublicKey)
	if imageURL == "" && imageSHA256 == "" && signature == "" && publicKey == "" {
		return nil
	}
	if imageURL == "" || imageSHA256 == "" {
		return fmt.Errorf("sandbox.image_url and sandbox.image_sha256 must be provided together")
	}
	parsed, err := url.Parse(imageURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return fmt.Errorf("sandbox.image_url must be an HTTPS URL without userinfo, query, or fragment")
	}
	if len(imageSHA256) != 64 {
		return fmt.Errorf("sandbox.image_sha256 must be a 64-character hexadecimal SHA-256 digest")
	}
	if _, err := hex.DecodeString(imageSHA256); err != nil {
		return fmt.Errorf("sandbox.image_sha256 must be a 64-character hexadecimal SHA-256 digest")
	}
	if (signature == "") != (publicKey == "") {
		return fmt.Errorf("sandbox.image_signature and sandbox.image_public_key must be provided together")
	}
	if signature != "" {
		decodedSignature, signatureErr := base64.StdEncoding.DecodeString(signature)
		decodedPublicKey, publicKeyErr := base64.StdEncoding.DecodeString(publicKey)
		if signatureErr != nil || len(decodedSignature) != 64 || publicKeyErr != nil || len(decodedPublicKey) != 32 {
			return fmt.Errorf("sandbox image signature and public key must be base64 Ed25519 values")
		}
	}
	return nil
}

func validateProvider(name string, provider ProviderConfig) error {
	switch provider.Type {
	case "openai":
		if provider.Protocol != "responses" && provider.Protocol != "chat_completions" {
			return fmt.Errorf("models.providers.%s.protocol must be responses or chat_completions", name)
		}
	case "anthropic":
		if provider.Protocol != "messages" {
			return fmt.Errorf("models.providers.%s.protocol must be messages", name)
		}
	default:
		return fmt.Errorf("models.providers.%s.type must be openai or anthropic", name)
	}
	parsed, err := url.Parse(provider.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("models.providers.%s.base_url must be an HTTP(S) URL", name)
	}
	return nil
}

func (cfg Config) Status() Status {
	status := Status{CampusConfigured: cfg.Campus.Key != "", DefaultProvider: cfg.Models.Default}
	for name, provider := range cfg.Models.Providers {
		if provider.APIKey != "" && provider.Model != "" {
			status.ConfiguredProviders = append(status.ConfiguredProviders, name)
		}
	}
	sort.Strings(status.ConfiguredProviders)
	return status
}

func (cfg Config) CampusAuthStatus() CampusAuthStatus {
	if strings.TrimSpace(cfg.Campus.Key) == "" {
		return CampusAuthStatus{
			State:                    CampusAuthNotConfigured,
			Configured:               false,
			Method:                   "none",
			DeviceAuthorization:      "unavailable",
			ServerClientRegistration: true,
			Notice:                   "请填写杭电校园 Key；Station 的正式设备授权客户端尚未注册。",
		}
	}
	return CampusAuthStatus{
		State:                    CampusAuthPATConfigured,
		Configured:               true,
		Method:                   "pat",
		DeviceAuthorization:      "unavailable",
		ServerClientRegistration: true,
		Notice:                   "当前使用本机配置的校园 Key；不会把它交给 Sandbox。Station 的正式设备授权客户端尚未注册。",
	}
}

type Status struct {
	CampusConfigured    bool     `json:"campusConfigured"`
	DefaultProvider     string   `json:"defaultProvider"`
	ConfiguredProviders []string `json:"configuredProviders"`
}
