package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

type StreamingProvider interface {
	CompleteStream(context.Context, Request, TextObserver) (Response, error)
}

type sseEvent struct {
	Name string
	Data string
}

const (
	maxStreamTextBytes    = 4 << 20
	maxStreamToolCalls    = 8
	maxStreamToolArgBytes = 64 << 10
)

func appendStreamText(builder *strings.Builder, value string, limit int, label string) error {
	if builder.Len()+len([]byte(value)) > limit {
		return fmt.Errorf("%s exceeds the %d-byte limit", label, limit)
	}
	builder.WriteString(value)
	return nil
}

func postStream(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, requestBody any) (*http.Response, error) {
	data, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("encode model streaming request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("create model streaming request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call model streaming provider: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2<<10))
		_ = response.Body.Close()
		return nil, fmt.Errorf("model provider returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return response, nil
}

func scanSSE(reader io.Reader, handle func(sseEvent) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4<<10), 1<<20)
	name := ""
	var data strings.Builder
	flush := func() error {
		if data.Len() == 0 && name == "" {
			return nil
		}
		err := handle(sseEvent{Name: name, Data: data.String()})
		name = ""
		data.Reset()
		return err
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read model streaming response: %w", err)
	}
	return flush()
}

type openAIStreamTool struct {
	ID        string
	Name      string
	Arguments strings.Builder
}

func (p *openAIChatProvider) CompleteStream(ctx context.Context, request Request, observer TextObserver) (Response, error) {
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
		Stream   bool              `json:"stream"`
	}{
		Model: p.config.Model, Messages: prependSystemMessage(request.SystemPrompt, messages), Tools: convertOpenAITools(request.Tools), Stream: true,
	}
	response, err := postStream(ctx, p.client, endpoint, map[string]string{"Authorization": "Bearer " + p.config.APIKey}, body)
	if err != nil {
		return Response{}, err
	}
	defer response.Body.Close()
	text := strings.Builder{}
	tools := map[int]*openAIStreamTool{}
	err = scanSSE(response.Body, func(event sseEvent) error {
		if event.Data == "[DONE]" {
			return nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(event.Data), &chunk); err != nil {
			return fmt.Errorf("decode OpenAI Chat streaming event: %w", err)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				if err := appendStreamText(&text, choice.Delta.Content, maxStreamTextBytes, "OpenAI Chat streamed text"); err != nil {
					return err
				}
				if observer != nil {
					observer(choice.Delta.Content)
				}
			}
			for _, delta := range choice.Delta.ToolCalls {
				call := tools[delta.Index]
				if call == nil {
					if len(tools) >= maxStreamToolCalls {
						return fmt.Errorf("OpenAI Chat stream returned more than %d tool calls", maxStreamToolCalls)
					}
					call = &openAIStreamTool{}
					tools[delta.Index] = call
				}
				if delta.ID != "" {
					call.ID = delta.ID
				}
				if delta.Function.Name != "" {
					call.Name = delta.Function.Name
				}
				if err := appendStreamText(&call.Arguments, delta.Function.Arguments, maxStreamToolArgBytes, "OpenAI Chat streamed tool arguments"); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return Response{}, err
	}
	toolCalls, err := openAIStreamTools(tools)
	if err != nil {
		return Response{}, err
	}
	if strings.TrimSpace(text.String()) == "" && len(toolCalls) == 0 {
		return Response{}, errors.New("OpenAI Chat Completions returned no streamed text or tool call")
	}
	return Response{Text: text.String(), ToolCalls: toolCalls}, nil
}

func openAIStreamTools(builders map[int]*openAIStreamTool) ([]ToolCall, error) {
	indices := make([]int, 0, len(builders))
	for index := range builders {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	tools := make([]ToolCall, 0, len(indices))
	for _, index := range indices {
		builder := builders[index]
		arguments := strings.TrimSpace(builder.Arguments.String())
		if arguments == "" {
			arguments = "{}"
		}
		if !json.Valid([]byte(arguments)) {
			return nil, errors.New("OpenAI Chat streaming returned invalid tool arguments")
		}
		tools = append(tools, ToolCall{ID: builder.ID, Name: builder.Name, Arguments: json.RawMessage(arguments)})
	}
	return tools, nil
}

type responsesStreamTool struct {
	ID        string
	Name      string
	Arguments strings.Builder
}

func (p *openAIResponsesProvider) CompleteStream(ctx context.Context, request Request, observer TextObserver) (Response, error) {
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
		Stream       bool             `json:"stream"`
	}{
		Model: p.config.Model, Instructions: request.SystemPrompt, Input: input, Tools: convertResponsesTools(request.Tools), Stream: true,
	}
	response, err := postStream(ctx, p.client, endpoint, map[string]string{"Authorization": "Bearer " + p.config.APIKey}, body)
	if err != nil {
		return Response{}, err
	}
	defer response.Body.Close()
	text := strings.Builder{}
	tools := map[string]*responsesStreamTool{}
	err = scanSSE(response.Body, func(event sseEvent) error {
		if event.Data == "[DONE]" {
			return nil
		}
		switch event.Name {
		case "response.output_text.delta":
			var payload struct {
				Delta string `json:"delta"`
			}
			if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
				return fmt.Errorf("decode OpenAI Responses text event: %w", err)
			}
			if err := appendStreamText(&text, payload.Delta, maxStreamTextBytes, "OpenAI Responses streamed text"); err != nil {
				return err
			}
			if observer != nil && payload.Delta != "" {
				observer(payload.Delta)
			}
		case "response.output_item.added":
			var payload struct {
				Item struct {
					Type   string `json:"type"`
					ID     string `json:"id"`
					CallID string `json:"call_id"`
					Name   string `json:"name"`
				} `json:"item"`
			}
			if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
				return fmt.Errorf("decode OpenAI Responses tool event: %w", err)
			}
			if payload.Item.Type == "function_call" {
				if _, exists := tools[payload.Item.ID]; !exists && len(tools) >= maxStreamToolCalls {
					return fmt.Errorf("OpenAI Responses stream returned more than %d tool calls", maxStreamToolCalls)
				}
				key := payload.Item.ID
				if key == "" {
					key = payload.Item.CallID
				}
				tools[key] = &responsesStreamTool{ID: payload.Item.CallID, Name: payload.Item.Name}
			}
		case "response.function_call_arguments.delta":
			var payload struct {
				ItemID string `json:"item_id"`
				Delta  string `json:"delta"`
			}
			if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
				return fmt.Errorf("decode OpenAI Responses arguments event: %w", err)
			}
			builder := tools[payload.ItemID]
			if builder == nil {
				builder = &responsesStreamTool{}
				tools[payload.ItemID] = builder
			}
			if err := appendStreamText(&builder.Arguments, payload.Delta, maxStreamToolArgBytes, "OpenAI Responses streamed tool arguments"); err != nil {
				return err
			}
		case "response.function_call_arguments.done":
			var payload struct {
				ItemID    string `json:"item_id"`
				Arguments string `json:"arguments"`
			}
			if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
				return fmt.Errorf("decode OpenAI Responses completed arguments: %w", err)
			}
			builder := tools[payload.ItemID]
			if builder == nil {
				builder = &responsesStreamTool{}
				tools[payload.ItemID] = builder
			}
			builder.Arguments.Reset()
			if err := appendStreamText(&builder.Arguments, payload.Arguments, maxStreamToolArgBytes, "OpenAI Responses streamed tool arguments"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Response{}, err
	}
	toolCalls := make([]ToolCall, 0, len(tools))
	keys := make([]string, 0, len(tools))
	for key := range tools {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		builder := tools[key]
		arguments := strings.TrimSpace(builder.Arguments.String())
		if arguments == "" {
			arguments = "{}"
		}
		if !json.Valid([]byte(arguments)) {
			return Response{}, errors.New("OpenAI Responses streaming returned invalid tool arguments")
		}
		toolCalls = append(toolCalls, ToolCall{ID: builder.ID, Name: builder.Name, Arguments: json.RawMessage(arguments)})
	}
	if strings.TrimSpace(text.String()) == "" && len(toolCalls) == 0 {
		return Response{}, errors.New("OpenAI Responses returned no streamed text or tool call")
	}
	return Response{Text: text.String(), ToolCalls: toolCalls}, nil
}

type anthropicStreamTool struct {
	ID        string
	Name      string
	Arguments strings.Builder
}

func (p *anthropicProvider) CompleteStream(ctx context.Context, request Request, observer TextObserver) (Response, error) {
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
		Stream    bool               `json:"stream"`
	}{
		Model: p.config.Model, MaxTokens: 1024, System: request.SystemPrompt, Messages: messages, Tools: convertAnthropicTools(request.Tools), Stream: true,
	}
	response, err := postStream(ctx, p.client, endpoint, map[string]string{"x-api-key": p.config.APIKey, "anthropic-version": defaultAnthropicVersion}, body)
	if err != nil {
		return Response{}, err
	}
	defer response.Body.Close()
	text := strings.Builder{}
	tools := map[int]*anthropicStreamTool{}
	err = scanSSE(response.Body, func(event sseEvent) error {
		var payload struct {
			Index        int `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		switch event.Name {
		case "content_block_start":
			if payload.ContentBlock.Type == "tool_use" {
				tools[payload.Index] = &anthropicStreamTool{ID: payload.ContentBlock.ID, Name: payload.ContentBlock.Name}
			}
		case "content_block_delta":
			if payload.Delta.Type == "text_delta" && payload.Delta.Text != "" {
				if err := appendStreamText(&text, payload.Delta.Text, maxStreamTextBytes, "Anthropic streamed text"); err != nil {
					return err
				}
				if observer != nil {
					observer(payload.Delta.Text)
				}
			}
			if payload.Delta.Type == "input_json_delta" && payload.Delta.PartialJSON != "" {
				builder := tools[payload.Index]
				if builder == nil {
					if len(tools) >= maxStreamToolCalls {
						return fmt.Errorf("Anthropic stream returned more than %d tool calls", maxStreamToolCalls)
					}
					builder = &anthropicStreamTool{}
					tools[payload.Index] = builder
				}
				if err := appendStreamText(&builder.Arguments, payload.Delta.PartialJSON, maxStreamToolArgBytes, "Anthropic streamed tool arguments"); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return Response{}, err
	}
	indices := make([]int, 0, len(tools))
	for index := range tools {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	toolCalls := make([]ToolCall, 0, len(indices))
	for _, index := range indices {
		builder := tools[index]
		arguments := strings.TrimSpace(builder.Arguments.String())
		if arguments == "" {
			arguments = "{}"
		}
		if !json.Valid([]byte(arguments)) {
			return Response{}, errors.New("Anthropic streaming returned invalid tool arguments")
		}
		toolCalls = append(toolCalls, ToolCall{ID: builder.ID, Name: builder.Name, Arguments: json.RawMessage(arguments)})
	}
	if strings.TrimSpace(text.String()) == "" && len(toolCalls) == 0 {
		return Response{}, errors.New("Anthropic Messages returned no streamed text or tool call")
	}
	return Response{Text: text.String(), ToolCalls: toolCalls}, nil
}
