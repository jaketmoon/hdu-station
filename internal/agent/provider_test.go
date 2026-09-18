package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/jaketmoon/hdu-station/internal/tools"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelChoosesToolsWithinBudget(t *testing.T) {
	for _, tc := range []struct {
		required string
		round    int
		force    bool
	}{
		{"fit_courses_to_schedule", 0, true}, {"", 0, false}, {"unregistered", 0, false}, {"fit_courses_to_schedule", 6, false},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]json.RawMessage
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Error("invalid model payload")
			}
			var choice struct {
				Type     string
				Function struct{ Name string }
			}
			_ = json.Unmarshal(body["tool_choice"], &choice)
			if (tc.round < 6) != (len(body["tools"]) > 0) {
				t.Error("tool budget not respected")
			}
			if len(body["tool_choice"]) > 0 {
				t.Error("forced an unavailable or budget-exhausted tool")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"已处理\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
		}))
		p := NewProvider(config.Model{BaseURL: server.URL, APIKey: "test", Name: "test"}, nil)
		p.state.round = tc.round
		tool, err := utils.InferTool("fit_courses_to_schedule", "test", tools.NewCampusSession(nil, nil).FitCourses)
		if err != nil {
			t.Fatal(err)
		}
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		p.tools = []*schema.ToolInfo{info}
		if _, err = p.Generate(context.Background(), []*schema.Message{schema.UserMessage("检查候选")}); err != nil {
			t.Fatal(err)
		}
		server.Close()
	}
}

func TestStreamToolCallsAndText(t *testing.T) {
	events := []Event{}
	p := NewProvider(config.Model{}, func(e Event) { events = append(events, e) })
	stream := `data: {"model":"deepseek-flash","choices":[{"delta":{"content":"我来看看","tool_calls":[{"index":0,"id":"call_1","function":{"name":"search_","arguments":"{\"query\":"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"courses","arguments":"\"通识选修\"}"}}]},"finish_reason":"tool_calls"}]}

data: [DONE]
`
	message, err := p.consume(strings.NewReader(stream))
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "我来看看" || len(message.ToolCalls) != 1 || message.ToolCalls[0].Function.Name != "search_courses" || message.ToolCalls[0].Function.Arguments != `{"query":"通识选修"}` {
		t.Fatalf("bad streamed message: %+v", message)
	}
	if len(events) != 1 || p.state.actualModel != "deepseek-flash" {
		t.Fatal("streaming metadata missing")
	}
}
func TestInterruptedAndOversizedStreamsAreNotSuccess(t *testing.T) {
	for name, stream := range map[string]string{
		"truncated":      `data: {"choices":[{"delta":{"content":"部分回答"}}]}` + "\n",
		"invalid":        "data: {\n\ndata: [DONE]\n",
		"too many calls": `data: {"choices":[{"delta":{"tool_calls":[{"index":99}]}}]}` + "\ndata: [DONE]\n",
		"length":         `data: {"choices":[{"delta":{"content":"部分回答"},"finish_reason":"length"}]}` + "\ndata: [DONE]\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewProvider(config.Model{}, nil).consume(strings.NewReader(stream))
			if err == nil {
				t.Fatal("incomplete stream accepted")
			}
		})
	}
}
func TestCredentialErrorsAreSafeAndRedirectsAreBlocked(t *testing.T) {
	leaked := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true; w.WriteHeader(200) }))
	defer target.Close()
	for _, status := range []int{401, 302} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer secret-sentinel" {
				t.Error("missing auth")
			}
			w.Header().Set("Location", target.URL)
			w.WriteHeader(status)
			fmt.Fprint(w, "secret-sentinel")
		}))
		p := NewProvider(config.Model{BaseURL: server.URL, APIKey: "secret-sentinel", Name: "deepseek-flash"}, nil)
		_, err := p.Generate(context.Background(), []*schema.Message{schema.UserMessage("你好")})
		if err == nil || strings.Contains(err.Error(), "secret-sentinel") {
			t.Fatal("unsafe model error")
		}
		server.Close()
	}
	if leaked {
		t.Fatal("credentials followed redirect")
	}
}
