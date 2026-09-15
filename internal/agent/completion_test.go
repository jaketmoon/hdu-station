package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/jaketmoon/hdu-station/internal/storage"
)

func TestSafeErrorKeepsCategoryWithoutUpstreamDetails(t *testing.T) {
	for _, message := range []string{"模型暂时不可用（HTTP 502）", "模型工具请求不完整", "模型生成中断，请重试"} {
		got := safeError(errors.New("upstream secret=test-secret wrapped: " + message))
		if !strings.Contains(got, message) || strings.Contains(got, "test-secret") {
			t.Fatal("lost safe error category or exposed upstream details")
		}
	}
	if strings.Contains(safeError(errors.New("unrecognized test-secret")), "test-secret") {
		t.Fatal("unrecognized upstream details leaked")
	}
	for source, expected := range map[string]string{
		"[LocalFunc] failed to unmarshal arguments: test-secret": "模型生成的查询参数格式不正确，请重试",
		"tool test-secret not found in toolsNode indexes":        "模型请求了不可用的查询工具，请重试",
		"test-secret exceeds max steps":                          "本轮查询步骤已达上限，请缩小问题范围后重试",
	} {
		if got := safeError(errors.New(source)); got != expected {
			t.Fatal("framework failure was not safely classified")
		}
	}
}

func TestPrematureShowDoesNotForceUnrequestedDiscovery(t *testing.T) {
	rounds := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rounds++
		w.Header().Set("Content-Type", "text/event-stream")
		if rounds == 1 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"early\",\"function\":{\"name\":\"show_course_results\",\"arguments\":\"{\\\"matchSchedule\\\":true}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n")
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"尚未找到可核实的信息。\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
		}
	}))
	defer server.Close()
	e := Engine{Model: config.Model{BaseURL: server.URL, APIKey: "test", Name: "test"}}
	r, err := e.Answer(context.Background(), []storage.Message{{Role: "user", State: "complete", Content: "按本人课表推荐影视音乐鉴赏"}}, nil)
	if err != nil || rounds != 2 || r.Text != "尚未找到可核实的信息。" {
		t.Fatalf("premature completion escaped guard: rounds=%d err=%v", rounds, err)
	}
}

func TestCurrentTermToolIsRegisteredAndCanShowItsSafeFailure(t *testing.T) {
	rounds := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rounds++
		name := "get_academic_term"
		if rounds > 1 {
			name = "show_course_results"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"term-%d\",\"function\":{\"name\":\"%s\",\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n", rounds, name)
	}))
	defer server.Close()
	engine := Engine{Model: config.Model{BaseURL: server.URL, APIKey: "test", Name: "test"}}
	result, err := engine.Answer(context.Background(), []storage.Message{{Role: "user", State: "complete", Content: "本学期是哪个学期？"}}, nil)
	if err != nil || rounds != 2 || !strings.Contains(result.Text, "尚未确认教务默认查询学期") || result.Searches != 0 || result.Reads != 0 {
		t.Fatalf("term-only tool did not finish: rounds=%d err=%v", rounds, err)
	}
}
