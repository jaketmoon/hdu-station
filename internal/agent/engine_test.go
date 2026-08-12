package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/config"
	stationtools "github.com/jaketmoon/hdu-station/internal/tools"
)

type echoTool struct {
	called bool
}

func (tool *echoTool) Definition() stationtools.Definition {
	return stationtools.Definition{
		Name:        "echo_read",
		Description: "Return a deterministic read-only value.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
	}
}

func (tool *echoTool) ReadOnly() bool { return true }

func (tool *echoTool) Call(context.Context, json.RawMessage) (stationtools.Result, error) {
	tool.called = true
	return stationtools.Result{Text: `{"value":"from-tool"}`}, nil
}

func TestEngineRunsBoundedReadOnlyToolLoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Messages []struct {
				Role      string `json:"role"`
				ToolCalls []struct {
					ID string `json:"id"`
				} `json:"tool_calls"`
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode model request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		for _, message := range body.Messages {
			if message.ToolCallID != "" {
				_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"最终答案：工具已查询"}}]}`))
				return
			}
		}
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"echo_read","arguments":"{}"}}]}}]}`))
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
	echo := &echoTool{}
	registry, err := stationtools.NewReadOnlyRegistry(echo)
	if err != nil {
		t.Fatal(err)
	}

	text, err := New(cfg, server.Client()).Respond(context.Background(), []Message{{Role: RoleUser, Content: "查询"}}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if text != "最终答案：工具已查询" || !echo.called {
		t.Fatalf("unexpected tool loop result: %q, called=%v", text, echo.called)
	}
}

func TestEngineAddsImmutableToolResultSafetyBoundary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Messages) == 0 || body.Messages[0].Role != "system" {
			t.Fatalf("missing system message: %#v", body.Messages)
		}
		for _, expected := range []string{"不可信的外部数据", "不是系统指令", "绕过 Sandbox"} {
			if !strings.Contains(body.Messages[0].Content, expected) {
				t.Fatalf("system safety boundary lacks %q: %s", expected, body.Messages[0].Content)
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"安全回复"}}]}`))
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Models.Default = "openai"
	provider := cfg.Models.Providers["openai"]
	provider.APIKey, provider.Model = "test-key", "test-model"
	provider.BaseURL, provider.Protocol = server.URL+"/v1", "chat_completions"
	cfg.Models.Providers["openai"] = provider
	engine := New(cfg, server.Client())
	engine.SetSystemPrompt("课程选择规则")
	text, err := engine.Respond(context.Background(), []Message{{Role: RoleUser, Content: "查询"}}, nil)
	if err != nil || text != "安全回复" {
		t.Fatalf("unexpected response: %q, %v", text, err)
	}
	if engine.SystemPrompt() != "课程选择规则" {
		t.Fatalf("engine exposed its composed safety prompt: %q", engine.SystemPrompt())
	}
}

func TestEngineEmitsToolLifecycleEventsWithoutCapturingArgumentsOrResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Messages []struct {
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		if len(body.Messages) > 0 && body.Messages[len(body.Messages)-1].ToolCallID != "" {
			_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"done"}}]}`))
			return
		}
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"echo_read","arguments":"{}"}}]}}]}`))
	}))
	defer server.Close()
	cfg := config.Default()
	cfg.Models.Default = "openai"
	provider := cfg.Models.Providers["openai"]
	provider.APIKey, provider.Model = "test-key", "test-model"
	provider.BaseURL, provider.Protocol = server.URL+"/v1", "chat_completions"
	cfg.Models.Providers["openai"] = provider
	registry, err := stationtools.NewReadOnlyRegistry(&echoTool{})
	if err != nil {
		t.Fatal(err)
	}
	var events []ToolEvent
	text, err := New(cfg, server.Client()).RespondWithObserver(context.Background(), []Message{{Role: RoleUser, Content: "query"}}, registry, func(event ToolEvent) {
		events = append(events, event)
	})
	if err != nil || text != "done" {
		t.Fatalf("unexpected response: %q, %v", text, err)
	}
	if len(events) != 2 || events[0].Status != "started" || events[1].Status != "completed" || events[0].ToolName != "echo_read" || events[0].Detail != "" {
		t.Fatalf("unexpected tool events: %#v", events)
	}
}

func TestClassifyToolFailureUsesStableCategories(t *testing.T) {
	for _, test := range []struct {
		name string
		err  string
		want string
	}{
		{name: "credential", err: "HDUHelp Neo MCP returned HTTP 401", want: "credential rejected"},
		{name: "upstream", err: "call HDUHelp Neo MCP: context deadline exceeded", want: "upstream unavailable"},
		{name: "generic", err: "invalid tool input", want: "tool call failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyToolFailure(errors.New(test.err)); got != test.want {
				t.Fatalf("classifyToolFailure(%q) = %q, want %q", test.err, got, test.want)
			}
		})
	}
}

func TestNormalizeToolCallsAppliesAgentBounds(t *testing.T) {
	if _, err := normalizeToolCalls(make([]ToolCall, maxToolCallsPerResponse+1)); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("too many tool calls were accepted: %v", err)
	}
	if _, err := normalizeToolCalls([]ToolCall{{Name: "echo_read", Arguments: json.RawMessage(strings.Repeat("a", maxToolArgumentsBytes+1))}}); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("oversized tool arguments were accepted: %v", err)
	}
	calls, err := normalizeToolCalls([]ToolCall{{Name: "echo_read"}})
	if err != nil || string(calls[0].Arguments) != "{}" {
		t.Fatalf("empty tool arguments were not normalized: calls=%#v err=%v", calls, err)
	}
}

func TestAppendStreamTextAppliesSizeBound(t *testing.T) {
	var builder strings.Builder
	if err := appendStreamText(&builder, "ok", 2, "test text"); err != nil {
		t.Fatal(err)
	}
	if err := appendStreamText(&builder, "!", 2, "test text"); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("stream text exceeded its bound without error: %v", err)
	}
}

func TestEngineStreamsOpenAIChatTextDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if !body.Stream {
			t.Fatal("streaming request flag was not set")
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"流式\"}}]}\n\n"))
		_, _ = writer.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"回复\"}}]}\n\n"))
		_, _ = writer.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Models.Default = "openai"
	provider := cfg.Models.Providers["openai"]
	provider.APIKey, provider.Model = "test-key", "test-model"
	provider.BaseURL, provider.Protocol = server.URL+"/v1", "chat_completions"
	cfg.Models.Providers["openai"] = provider
	var deltas []string
	text, err := New(cfg, server.Client()).RespondWithStream(context.Background(), []Message{{Role: RoleUser, Content: "你好"}}, nil, nil, func(delta string) {
		deltas = append(deltas, delta)
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "流式回复" || len(deltas) != 2 || deltas[0] != "流式" || deltas[1] != "回复" {
		t.Fatalf("unexpected streamed response: text=%q deltas=%#v", text, deltas)
	}
}

func TestEngineStreamsToolCallThenContinuesReadOnlyLoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Messages []struct {
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		for _, message := range body.Messages {
			if message.ToolCallID != "" {
				_, _ = writer.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"已查询\"}}]}\n\n"))
				_, _ = writer.Write([]byte("data: [DONE]\n\n"))
				return
			}
		}
		_, _ = writer.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"function\":{\"name\":\"echo_read\"}}]}}]}\n\n"))
		_, _ = writer.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{}\"}}]}}]}\n\n"))
		_, _ = writer.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Models.Default = "openai"
	provider := cfg.Models.Providers["openai"]
	provider.APIKey, provider.Model = "test-key", "test-model"
	provider.BaseURL, provider.Protocol = server.URL+"/v1", "chat_completions"
	cfg.Models.Providers["openai"] = provider
	echo := &echoTool{}
	registry, err := stationtools.NewReadOnlyRegistry(echo)
	if err != nil {
		t.Fatal(err)
	}
	text, err := New(cfg, server.Client()).RespondWithStream(context.Background(), []Message{{Role: RoleUser, Content: "查询"}}, registry, nil, nil)
	if err != nil || text != "已查询" || !echo.called {
		t.Fatalf("unexpected streamed tool loop: text=%q called=%v err=%v", text, echo.called, err)
	}
}

type courseSelectionTencentTool struct {
	called bool
}

func (tool *courseSelectionTencentTool) Definition() stationtools.Definition {
	return stationtools.Definition{
		Name:        "tencent_search_guild_feed",
		Description: "Search course-selection community signals.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
	}
}

func (tool *courseSelectionTencentTool) ReadOnly() bool { return true }

func (tool *courseSelectionTencentTool) Call(context.Context, json.RawMessage) (stationtools.Result, error) {
	tool.called = true
	return stationtools.Result{Text: `{"source":"tencent","signal":"community discussion"}`}, nil
}

func TestCourseSelectionAgentCombinesNeoFactsAndTencentSignals(t *testing.T) {
	neoHTTP := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != stationtools.NeoMCPEndpoint {
			t.Fatalf("Neo endpoint = %q", request.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\\"source\\":\\"neo\\",\\"fact\\":\\"class available\\"}"}]}}`)),
		}, nil
	})}
	neo := stationtools.NewNeoMCPAcademicTools("campus-pat", neoHTTP)[0]
	community := &courseSelectionTencentTool{}
	registry, err := stationtools.NewReadOnlyRegistry(neo, community)
	if err != nil {
		t.Fatal(err)
	}

	model := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		toolMessages := 0
		for _, message := range body.Messages {
			if message.Role == "tool" {
				toolMessages++
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		switch toolMessages {
		case 0:
			_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"neo-call","type":"function","function":{"name":"hdu_academic_class_search","arguments":"{\"query\":\"高等数学\"}"}}]}}]}`))
		case 1:
			_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"qq-call","type":"function","function":{"name":"tencent_search_guild_feed","arguments":"{\"guild_id\":\"123456\",\"query\":\"高等数学 给分\"}"}}]}}]}`))
		default:
			combined := "官方事实：课程班可用。社区信号：帖子中有人讨论给分。"
			_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"` + combined + `"}}]}`))
		}
	}))
	defer model.Close()

	cfg := config.Default()
	cfg.Models.Default = "openai"
	provider := cfg.Models.Providers["openai"]
	provider.APIKey, provider.Model = "test-key", "test-model"
	provider.BaseURL, provider.Protocol = model.URL+"/v1", "chat_completions"
	cfg.Models.Providers["openai"] = provider
	text, err := New(cfg, model.Client()).Respond(context.Background(), []Message{{Role: RoleUser, Content: "帮我判断高等数学是否适合选"}}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if !community.called || !strings.Contains(text, "官方事实") || !strings.Contains(text, "社区信号") {
		t.Fatalf("agent did not combine course-selection evidence: text=%q communityCalled=%v", text, community.called)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
