package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/config"
)

func TestEngineUsesOpenAIChatCompletions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization header was not set")
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "test-model" || len(body.Messages) != 2 || body.Messages[0].Role != "system" || !strings.Contains(body.Messages[0].Content, "course policy") || !strings.Contains(body.Messages[0].Content, "不可信的外部数据") || body.Messages[1].Content != "hello" {
			t.Errorf("unexpected request: %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi from openai"}}]}`))
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Models.Default = "openai"
	provider := cfg.Models.Providers["openai"]
	provider.APIKey = "test-key"
	provider.Model = "test-model"
	provider.BaseURL = server.URL + "/v1"
	provider.Protocol = "chat_completions"
	cfg.Models.Providers["openai"] = provider

	text, err := New(cfg, server.Client()).Complete(context.Background(), Request{
		SystemPrompt: "course policy",
		Messages:     []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "hi from openai" {
		t.Fatalf("response = %q", text)
	}
}

func TestEngineUsesOpenAIResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/responses" {
			t.Errorf("path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"hi from responses"}]}]}`))
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Models.Default = "openai"
	provider := cfg.Models.Providers["openai"]
	provider.APIKey = "test-key"
	provider.Model = "test-model"
	provider.BaseURL = server.URL + "/v1"
	provider.Protocol = "responses"
	cfg.Models.Providers["openai"] = provider

	text, err := New(cfg, server.Client()).Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "hi from responses" {
		t.Fatalf("response = %q", text)
	}
}

func TestEngineUsesAnthropicMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/messages" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("x-api-key") != "test-key" || request.Header.Get("anthropic-version") == "" {
			t.Errorf("Anthropic headers were not set")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"content":[{"type":"text","text":"hi from anthropic"}]}`))
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Models.Default = "anthropic"
	provider := cfg.Models.Providers["anthropic"]
	provider.APIKey = "test-key"
	provider.Model = "test-model"
	provider.BaseURL = server.URL
	cfg.Models.Providers["anthropic"] = provider

	text, err := New(cfg, server.Client()).Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "hi from anthropic" {
		t.Fatalf("response = %q", text)
	}
}

func TestEngineFailsWithoutConfiguredProvider(t *testing.T) {
	engine := New(config.Default(), nil)
	_, err := engine.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err == nil || !strings.Contains(err.Error(), "no model provider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEngineRejectsModelProviderRedirects(t *testing.T) {
	redirected := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/redirected" {
			redirected = true
		}
		http.Redirect(writer, request, "/redirected", http.StatusFound)
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Models.Default = "openai"
	provider := cfg.Models.Providers["openai"]
	provider.APIKey, provider.Model = "test-key", "test-model"
	provider.BaseURL, provider.Protocol = server.URL+"/v1", "chat_completions"
	cfg.Models.Providers["openai"] = provider
	_, err := New(cfg, server.Client()).Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hello"}}})
	if err == nil || !strings.Contains(err.Error(), "redirects are disabled") {
		t.Fatalf("model redirect was not rejected: %v", err)
	}
	if redirected {
		t.Fatal("model client followed a redirect")
	}
}

func TestEndpointURLDoesNotDuplicateV1Path(t *testing.T) {
	endpoint, err := endpointURL("https://example.test/v1", "v1/messages")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "https://example.test/v1/messages" {
		t.Fatalf("endpoint = %q", endpoint)
	}
}
