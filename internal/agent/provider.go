package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/jaketmoon/hdu-station/internal/config"
)

type Event struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}
type runState struct {
	round       int
	actualModel string
}
type Provider struct {
	config      config.Model
	client      *http.Client
	tools       []*schema.ToolInfo
	emit        func(Event)
	state       *runState
	canUseTools func() bool
}

func NewProvider(c config.Model, emit func(Event)) *Provider {
	if emit == nil {
		emit = func(Event) {}
	}
	return &Provider{config: c, emit: emit, state: &runState{}, client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("不允许模型接口重定向") }}}
}
func (p *Provider) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	copy := *p
	copy.tools = append([]*schema.ToolInfo(nil), tools...)
	return &copy, nil
}
func (p *Provider) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](1)
	go func() {
		defer writer.Close()
		message, err := p.Generate(ctx, input, opts...)
		writer.Send(message, err)
	}()
	return reader, nil
}

// Generate streams HTTP deltas to the desktop while Eino owns the tool loop.
func (p *Provider) Generate(ctx context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*1e9)
	defer cancel()
	p.state.round++
	messages := make([]map[string]any, 0, len(input))
	for _, m := range input {
		v := map[string]any{"role": m.Role, "content": m.Content}
		if len(m.ToolCalls) > 0 {
			v["tool_calls"] = m.ToolCalls
		}
		if m.ToolCallID != "" {
			v["tool_call_id"] = m.ToolCallID
		}
		messages = append(messages, v)
	}
	payload := map[string]any{"model": p.config.Name, "messages": messages, "stream": true, "max_tokens": 4096, "thinking": map[string]string{"type": "disabled"}}
	if p.state.round <= 6 && (p.canUseTools == nil || p.canUseTools()) {
		definitions := []any{}
		for _, t := range p.tools {
			params, err := t.ParamsOneOf.ToJSONSchema()
			if err != nil {
				return nil, errors.New("工具参数定义不正确")
			}
			definitions = append(definitions, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Desc, "parameters": params}})
		}
		if len(definitions) > 0 {
			payload["tools"] = definitions
		}
	} else {
		messages = append(messages, map[string]any{"role": "system", "content": "请根据已经读到的资料完成回答；如果资料不足，请如实说明。"})
		payload["messages"] = messages
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("无法构造模型请求")
	}
	if len(data) > 512<<10 {
		return nil, errors.New("这轮对话内容过长，请开启新对话")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(p.config.BaseURL, "/")+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("模型地址不正确")
	}
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	p.emit(Event{Kind: "reset"})
	p.emit(Event{Kind: "status", Text: "正在整理思路…"})
	response, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("模型连接失败，请检查网络和设置")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		switch response.StatusCode {
		case 401, 403:
			return nil, errors.New("模型 API Key 无效或没有访问权限，请检查设置")
		case 429:
			return nil, errors.New("模型请求限流，请稍后重试")
		default:
			return nil, fmt.Errorf("模型暂时不可用（HTTP %d）", response.StatusCode)
		}
	}
	return p.consume(response.Body)
}
func (p *Provider) consume(body io.Reader) (*schema.Message, error) {
	result := schema.AssistantMessage("", nil)
	scanner := bufio.NewScanner(io.LimitReader(body, 2<<20))
	scanner.Buffer(make([]byte, 8192), 256<<10)
	calls := map[int]*schema.ToolCall{}
	done := false
	finish := ""
	size := 0
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			done = true
			break
		}
		var chunk struct {
			Model   string          `json:"model"`
			Error   json.RawMessage `json:"error"`
			Choices []struct {
				Finish string `json:"finish_reason"`
				Delta  struct {
					Content string `json:"content"`
					Calls   []struct {
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
		if json.Unmarshal([]byte(data), &chunk) != nil {
			return nil, errors.New("模型返回了无法解析的响应")
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return nil, errors.New("模型生成中断，请重试")
		}
		if chunk.Model != "" {
			p.state.actualModel = chunk.Model
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if choice.Finish != "" {
			finish = choice.Finish
		}
		text := choice.Delta.Content
		size += len(text)
		if size > 64<<10 {
			return nil, errors.New("回答超过长度限制，请缩小问题范围")
		}
		result.Content += text
		if text != "" {
			p.emit(Event{Kind: "delta", Text: text})
		}
		for _, delta := range choice.Delta.Calls {
			if delta.Index < 0 || delta.Index >= 6 {
				return nil, errors.New("模型请求了过多工具")
			}
			call := calls[delta.Index]
			if call == nil {
				call = &schema.ToolCall{Type: "function"}
				calls[delta.Index] = call
			}
			call.ID += delta.ID
			call.Function.Name += delta.Function.Name
			call.Function.Arguments += delta.Function.Arguments
			if len(call.ID) > 128 || len(call.Function.Name) > 128 || len(call.Function.Arguments) > 16<<10 {
				return nil, errors.New("工具请求超过长度限制")
			}
		}
	}
	if scanner.Err() != nil || !done {
		return nil, errors.New("模型连接中断，可以重新发送问题")
	}
	if finish == "length" {
		return nil, errors.New("回答达到长度上限，请缩小问题范围后重试")
	}
	for i := 0; i < len(calls); i++ {
		call := calls[i]
		if call == nil || call.ID == "" || !json.Valid([]byte(call.Function.Arguments)) {
			return nil, errors.New("模型工具请求不完整")
		}
		result.ToolCalls = append(result.ToolCalls, *call)
	}
	if result.Content == "" && len(result.ToolCalls) == 0 {
		return nil, errors.New("模型没有返回内容，请重试")
	}
	return result, nil
}
