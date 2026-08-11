package config

import (
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

type WebSearchConfig struct {
	Provider  string `yaml:"provider" json:"provider"`
	BraveKey  string `yaml:"brave_api_key,omitempty" json:"brave_api_key,omitempty"`
	TavilyKey string `yaml:"tavily_api_key,omitempty" json:"tavily_api_key,omitempty"`
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

type Status struct {
	CampusConfigured    bool     `json:"campusConfigured"`
	DefaultProvider     string   `json:"defaultProvider"`
	ConfiguredProviders []string `json:"configuredProviders"`
}
