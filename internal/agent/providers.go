package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/jaketmoon/hdu-station/internal/config"
)

const (
	defaultAnthropicVersion = "2023-06-01"
	maxResponseBytes        = 4 << 20
)

func newProvider(provider config.ProviderConfig, client *http.Client) Provider {
	switch {
	case provider.Type == "openai" && provider.Protocol == "chat_completions":
		return &openAIChatProvider{config: provider, client: client}
	case provider.Type == "openai" && provider.Protocol == "responses":
		return &openAIResponsesProvider{config: provider, client: client}
	case provider.Type == "anthropic" && provider.Protocol == "messages":
		return &anthropicProvider{config: provider, client: client}
	default:
		return &unsupportedProvider{config: provider}
	}
}

type unsupportedProvider struct {
	config config.ProviderConfig
}

func (p *unsupportedProvider) Complete(context.Context, Request) (Response, error) {
	return Response{}, fmt.Errorf("model provider protocol %q/%q is not supported", p.config.Type, p.config.Protocol)
}

type openAIChatProvider struct {
	config config.ProviderConfig
	client *http.Client
}

func (p *openAIChatProvider) Complete(ctx context.Context, request Request) (Response, error) {
	if err := validateProviderCredentials(p.config); err != nil {
		return Response{}, err
	}
	endpoint, err := endpointURL(p.config.BaseURL, "chat/completions")
	if err != nil {
		return Response{}, err
	}
	messages, err := convertOpenAIChatMessages(request.Messages)
	if err != nil {
		return Response{}, err
	}
	body := struct {
		Model    string            `json:"model"`
		Messages []providerMessage `json:"messages"`
		Tools    []openAITool      `json:"tools,omitempty"`
	}{
		Model:    p.config.Model,
		Messages: prependSystemMessage(request.SystemPrompt, messages),
		Tools:    convertOpenAITools(request.Tools),
	}
	var response struct {
		Choices []struct {
			Message providerMessage `json:"message"`
		} `json:"choices"`
	}
	if err := postJSON(ctx, p.client, endpoint, map[string]string{
		"Authorization": "Bearer " + p.config.APIKey,
	}, body, &response); err != nil {
		return Response{}, err
	}
	if len(response.Choices) == 0 {
		return Response{}, errors.New("OpenAI Chat Completions returned no message")
	}
	toolCalls, err := convertOpenAIToolCalls(response.Choices[0].Message.ToolCalls)
	if err != nil {
		return Response{}, err
	}
	if len(toolCalls) == 0 && strings.TrimSpace(response.Choices[0].Message.Content) == "" {
		return Response{}, errors.New("OpenAI Chat Completions returned no text or tool call")
	}
	return Response{Text: response.Choices[0].Message.Content, ToolCalls: toolCalls}, nil
}

type openAIResponsesProvider struct {
	config config.ProviderConfig
	client *http.Client
}

func (p *openAIResponsesProvider) Complete(ctx context.Context, request Request) (Response, error) {
	if err := validateProviderCredentials(p.config); err != nil {
		return Response{}, err
	}
	endpoint, err := endpointURL(p.config.BaseURL, "responses")
	if err != nil {
		return Response{}, err
	}
	input, err := convertResponsesInputs(request.Messages)
	if err != nil {
		return Response{}, err
	}
	body := struct {
		Model        string           `json:"model"`
		Instructions string           `json:"instructions,omitempty"`
		Input        []responsesInput `json:"input"`
		Tools        []responsesTool  `json:"tools,omitempty"`
	}{
		Model:        p.config.Model,
		Instructions: request.SystemPrompt,
		Input:        input,
		Tools:        convertResponsesTools(request.Tools),
	}
	var response responsesResponse
	if err := postJSON(ctx, p.client, endpoint, map[string]string{
		"Authorization": "Bearer " + p.config.APIKey,
	}, body, &response); err != nil {
		return Response{}, err
	}
	toolCalls := make([]ToolCall, 0)
	text := strings.TrimSpace(response.OutputText)
	for _, output := range response.Output {
		switch output.Type {
		case "function_call":
			arguments := json.RawMessage(output.Arguments)
			if len(arguments) == 0 {
				arguments = json.RawMessage(`{}`)
			}
			if !json.Valid(arguments) {
				return Response{}, errors.New("OpenAI Responses returned invalid tool arguments")
			}
			toolCalls = append(toolCalls, ToolCall{ID: output.CallID, Name: output.Name, Arguments: arguments})
		case "message":
			for _, content := range output.Content {
				if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
					text = content.Text
				}
			}
		}
	}
	if text == "" && len(toolCalls) == 0 {
		return Response{}, errors.New("OpenAI Responses returned no output text or tool call")
	}
	return Response{Text: text, ToolCalls: toolCalls}, nil
}

type anthropicProvider struct {
	config config.ProviderConfig
	client *http.Client
}

func (p *anthropicProvider) Complete(ctx context.Context, request Request) (Response, error) {
	if err := validateProviderCredentials(p.config); err != nil {
		return Response{}, err
	}
	endpoint, err := endpointURL(p.config.BaseURL, "v1/messages")
	if err != nil {
		return Response{}, err
	}
	messages, err := convertAnthropicMessages(request.Messages)
	if err != nil {
		return Response{}, err
	}
	body := struct {
		Model     string             `json:"model"`
		MaxTokens int                `json:"max_tokens"`
		System    string             `json:"system,omitempty"`
		Messages  []anthropicMessage `json:"messages"`
		Tools     []anthropicTool    `json:"tools,omitempty"`
	}{
		Model:     p.config.Model,
		MaxTokens: 1024,
		System:    request.SystemPrompt,
		Messages:  messages,
		Tools:     convertAnthropicTools(request.Tools),
	}
	var response anthropicResponse
	if err := postJSON(ctx, p.client, endpoint, map[string]string{
		"x-api-key":         p.config.APIKey,
		"anthropic-version": defaultAnthropicVersion,
	}, body, &response); err != nil {
		return Response{}, err
	}
	toolCalls := make([]ToolCall, 0)
	text := ""
	for _, content := range response.Content {
		switch content.Type {
		case "text":
			if strings.TrimSpace(content.Text) != "" {
				text += content.Text
			}
		case "tool_use":
			arguments := content.Input
			if len(arguments) == 0 {
				arguments = json.RawMessage(`{}`)
			}
			if !json.Valid(arguments) {
				return Response{}, errors.New("Anthropic Messages returned invalid tool arguments")
			}
			toolCalls = append(toolCalls, ToolCall{ID: content.ID, Name: content.Name, Arguments: arguments})
		}
	}
	if strings.TrimSpace(text) == "" && len(toolCalls) == 0 {
		return Response{}, errors.New("Anthropic Messages returned no text or tool call")
	}
	return Response{Text: text, ToolCalls: toolCalls}, nil
}

type providerMessage struct {
	Role       Role             `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type responsesInput struct {
	Type      string             `json:"type"`
	Role      Role               `json:"role,omitempty"`
	Content   []responsesContent `json:"content,omitempty"`
	CallID    string             `json:"call_id,omitempty"`
	Output    string             `json:"output,omitempty"`
	Name      string             `json:"name,omitempty"`
	Arguments string             `json:"arguments,omitempty"`
}

type responsesContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type responsesResponse struct {
	OutputText string `json:"output_text"`
	Output     []struct {
		Type      string             `json:"type"`
		CallID    string             `json:"call_id"`
		Name      string             `json:"name"`
		Arguments string             `json:"arguments"`
		Content   []responsesContent `json:"content"`
	} `json:"output"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicResponse struct {
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
}

func convertOpenAIChatMessages(messages []Message) ([]providerMessage, error) {
	converted := make([]providerMessage, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case RoleUser, RoleAssistant:
			converted = append(converted, providerMessage{
				Role:      message.Role,
				Content:   message.Content,
				ToolCalls: convertOpenAIToolCallsForMessage(message.ToolCalls),
			})
		case RoleTool:
			if message.ToolCallID == "" {
				return nil, errors.New("tool message is missing a tool call ID")
			}
			converted = append(converted, providerMessage{Role: RoleTool, Content: message.Content, ToolCallID: message.ToolCallID})
		default:
			return nil, fmt.Errorf("message role %q is not supported", message.Role)
		}
	}
	return converted, nil
}

func prependSystemMessage(prompt string, messages []providerMessage) []providerMessage {
	if strings.TrimSpace(prompt) == "" {
		return messages
	}
	return append([]providerMessage{{Role: RoleSystem, Content: prompt}}, messages...)
}

func convertOpenAIToolCallsForMessage(calls []ToolCall) []openAIToolCall {
	converted := make([]openAIToolCall, 0, len(calls))
	for _, call := range calls {
		converted = append(converted, openAIToolCall{
			ID:   call.ID,
			Type: "function",
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: call.Name, Arguments: string(call.Arguments)},
		})
	}
	return converted
}

func convertOpenAIToolCalls(calls []openAIToolCall) ([]ToolCall, error) {
	converted := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		arguments := json.RawMessage(call.Function.Arguments)
		if len(arguments) == 0 {
			arguments = json.RawMessage(`{}`)
		}
		if !json.Valid(arguments) {
			return nil, errors.New("OpenAI Chat Completions returned invalid tool arguments")
		}
		converted = append(converted, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: arguments})
	}
	return converted, nil
}

func convertOpenAITools(definitions []ToolDefinition) []openAITool {
	converted := make([]openAITool, 0, len(definitions))
	for _, definition := range definitions {
		converted = append(converted, openAITool{Type: "function", Function: openAIFunction{
			Name: definition.Name, Description: definition.Description, Parameters: definition.Parameters,
		}})
	}
	return converted
}

func convertResponsesInputs(messages []Message) ([]responsesInput, error) {
	converted := make([]responsesInput, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case RoleUser:
			converted = append(converted, responsesInput{Type: "message", Role: RoleUser, Content: []responsesContent{{Type: "input_text", Text: message.Content}}})
		case RoleAssistant:
			if strings.TrimSpace(message.Content) != "" {
				converted = append(converted, responsesInput{Type: "message", Role: RoleAssistant, Content: []responsesContent{{Type: "output_text", Text: message.Content}}})
			}
			for _, call := range message.ToolCalls {
				converted = append(converted, responsesInput{Type: "function_call", CallID: call.ID, Name: call.Name, Arguments: string(call.Arguments)})
			}
		case RoleTool:
			if message.ToolCallID == "" {
				return nil, errors.New("tool message is missing a tool call ID")
			}
			converted = append(converted, responsesInput{Type: "function_call_output", CallID: message.ToolCallID, Output: message.Content})
		default:
			return nil, fmt.Errorf("message role %q is not supported", message.Role)
		}
	}
	return converted, nil
}

func convertResponsesTools(definitions []ToolDefinition) []responsesTool {
	converted := make([]responsesTool, 0, len(definitions))
	for _, definition := range definitions {
		converted = append(converted, responsesTool{Type: "function", Name: definition.Name, Description: definition.Description, Parameters: definition.Parameters})
	}
	return converted
}

func convertAnthropicMessages(messages []Message) ([]anthropicMessage, error) {
	converted := make([]anthropicMessage, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case RoleUser:
			converted = append(converted, anthropicMessage{Role: "user", Content: message.Content})
		case RoleAssistant:
			if len(message.ToolCalls) == 0 {
				converted = append(converted, anthropicMessage{Role: "assistant", Content: message.Content})
				continue
			}
			content := make([]anthropicContent, 0, len(message.ToolCalls)+1)
			if strings.TrimSpace(message.Content) != "" {
				content = append(content, anthropicContent{Type: "text", Text: message.Content})
			}
			for _, call := range message.ToolCalls {
				content = append(content, anthropicContent{Type: "tool_use", ID: call.ID, Name: call.Name, Input: call.Arguments})
			}
			converted = append(converted, anthropicMessage{Role: "assistant", Content: content})
		case RoleTool:
			if message.ToolCallID == "" {
				return nil, errors.New("tool message is missing a tool call ID")
			}
			converted = append(converted, anthropicMessage{Role: "user", Content: []anthropicContent{{Type: "tool_result", ToolUseID: message.ToolCallID, Content: message.Content}}})
		default:
			return nil, fmt.Errorf("message role %q is not supported", message.Role)
		}
	}
	return converted, nil
}

func convertAnthropicTools(definitions []ToolDefinition) []anthropicTool {
	converted := make([]anthropicTool, 0, len(definitions))
	for _, definition := range definitions {
		converted = append(converted, anthropicTool{Name: definition.Name, Description: definition.Description, InputSchema: definition.Parameters})
	}
	return converted
}

func validateProviderCredentials(provider config.ProviderConfig) error {
	if strings.TrimSpace(provider.APIKey) == "" {
		return errors.New("model provider API key is not configured")
	}
	if strings.TrimSpace(provider.Model) == "" {
		return errors.New("model provider model is not configured")
	}
	return nil
}

func endpointURL(base, suffix string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("model provider base URL must be an HTTP(S) URL")
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	suffix = strings.TrimLeft(suffix, "/")
	if strings.HasSuffix(basePath, "/v1") && strings.HasPrefix(suffix, "v1/") {
		suffix = strings.TrimPrefix(suffix, "v1/")
	}
	parsed.Path = basePath + "/" + suffix
	parsed.RawPath = ""
	return parsed.String(), nil
}

func postJSON(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, requestBody, responseBody any) error {
	data, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("encode model request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create model request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("call model provider: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2<<10))
		return fmt.Errorf("model provider returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if err := decoder.Decode(responseBody); err != nil {
		return fmt.Errorf("decode model response: %w", err)
	}
	return nil
}
