package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/jaketmoon/hdu-station/internal/storage"
	"github.com/jaketmoon/hdu-station/internal/tools"
)

func TestFollowupKeepsPreviouslyReadCitationWithoutSearchingAgain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"model\":\"deepseek-flash\",\"choices\":[{\"delta\":{\"content\":\"之前读到的是期末论文，见 https://pd.qq.com/s/previous\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
	}))
	defer server.Close()
	engine := Engine{Model: config.Model{BaseURL: server.URL, APIKey: "test-model-key", Name: "deepseek-flash"}, Client: tools.NewClient(t.TempDir())}
	result, err := engine.Answer(context.Background(), []storage.Message{
		{Role: "assistant", State: "complete", Content: "这门课期末交论文。[原帖](https://pd.qq.com/s/previous)"},
		{Role: "user", State: "complete", Content: "再提醒我一下它怎么考核？"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reads != 0 || !strings.Contains(result.Text, "https://pd.qq.com/s/previous") || strings.Contains(result.Text, "未核实") {
		t.Fatal("follow-up lost a previously verified citation")
	}
}

func TestCourseCategoryRemainsUnavailableWithScopedFavoriteTool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct{ Role, Content string }
			Tools    []struct{ Function struct{ Name string } }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		allowed := map[string]bool{"search_courses": true, "read_course_posts": true, "get_academic_term": true, "check_course_offerings": true, "fit_courses_to_schedule": true, "show_course_results": true, "manage_course_collection": true}
		if len(request.Tools) != len(allowed) {
			t.Error("unexpected tool registration")
		}
		for _, tool := range request.Tools {
			if !allowed[tool.Function.Name] {
				t.Error("agent exposed a tool outside the authorized operations")
			}
		}
		if len(request.Messages) == 0 || request.Messages[0].Role != "system" || !strings.Contains(request.Messages[0].Content, "当前未接入课程分类") {
			t.Error("prompt still implies a campus login enables course classification")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"model\":\"test\",\"choices\":[{\"delta\":{\"content\":\"目前暂不支持核实课程类别，请以教务系统为准。\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
	}))
	defer server.Close()
	engine := Engine{Model: config.Model{BaseURL: server.URL, APIKey: "test", Name: "test"}}
	result, err := engine.Answer(context.Background(), []storage.Message{
		{Role: "assistant", State: "complete", Content: "之前查询过浙江传统文化。"},
		{Role: "user", State: "complete", Content: "已登录校园账号，上面的课是什么类别？"},
	}, nil)
	if err != nil || result.Searches != 0 || result.Reads != 0 || !strings.Contains(result.Text, "暂不支持") {
		t.Fatal("unavailable course query did not return through the normal answer path")
	}
}

func TestXiaohongshuSearchReadAndCitationThroughAgent(t *testing.T) {
	const noteID = "66abcdef1234567890abcdef"
	reads := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads++
		switch r.URL.Path {
		case "/api/v1/login/status":
			fmt.Fprint(w, `{"success":true,"data":{"is_logged_in":true}}`)
		case "/api/v1/feeds/search":
			fmt.Fprint(w, `{"success":true,"data":{"feeds":[{"id":"`+noteID+`","xsecToken":"signature-private-sentinel","modelType":"note","noteCard":{"displayTitle":"杭电选修"}}]}}`)
		case "/api/v1/feeds/detail":
			fmt.Fprint(w, `{"success":true,"data":{"data":{"note":{"noteId":"`+noteID+`","title":"杭电选修","desc":"考核交论文","user":{"userId":"author-private-sentinel"}},"comments":{"list":[],"hasMore":false}}}}`)
		default:
			t.Error("unexpected source operation")
			w.WriteHeader(404)
		}
	}))
	defer source.Close()
	rounds := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, _ := io.ReadAll(r.Body)
		if strings.Contains(string(request), "private-sentinel") {
			t.Error("source credentials or author reached model")
		}
		var delta any
		finish := "tool_calls"
		switch rounds {
		case 0:
			if !strings.Contains(string(request), "小红书（xiaohongshu）") {
				t.Error("configured source missing from agent prompt")
			}
			delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "search", "type": "function", "function": map[string]string{"name": "search_courses", "arguments": `{"query":"选修","source":"xiaohongshu"}`}}}}
		case 1:
			delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "read", "type": "function", "function": map[string]string{"name": "read_course_posts", "arguments": `{"posts":["post-1"]}`}}}}
		default:
			delta, finish = map[string]string{"content": "这门课有同学提到交论文。[原帖](post-1)"}, "stop"
		}
		rounds++
		frame, _ := json.Marshal(map[string]any{"model": "deepseek-flash", "choices": []any{map[string]any{"delta": delta, "finish_reason": finish}}})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n", frame)
	}))
	defer model.Close()
	engine := Engine{Model: config.Model{BaseURL: model.URL, APIKey: "model-key", Name: "deepseek-flash"}, Client: tools.NewClient(t.TempDir()), Sources: config.Sources{Xiaohongshu: config.Xiaohongshu{Enabled: true, BaseURL: source.URL, AuthToken: "auth-private-sentinel"}}}
	result, err := engine.Answer(context.Background(), []storage.Message{{Role: "user", State: "complete", Content: "小红书有什么杭电选修体验？"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rounds != 3 || reads != 3 || result.Searches != 1 || result.Reads != 1 || result.CampusCalls != 0 || !strings.Contains(result.Text, "https://www.xiaohongshu.com/explore/"+noteID) || strings.Contains(result.Text, "private-sentinel") {
		t.Fatal("agent source search/read/citation pipeline incomplete")
	}
}

func TestPrematureCampusDisplayKeepsReadCommunityAnswer(t *testing.T) {
	const noteID = "66abcdef1234567890abcdef"
	reads := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads++
		switch r.URL.Path {
		case "/api/v1/login/status":
			fmt.Fprint(w, `{"success":true,"data":{"is_logged_in":true}}`)
		case "/api/v1/feeds/search":
			fmt.Fprint(w, `{"success":true,"data":{"feeds":[{"id":"`+noteID+`","xsecToken":"signature-private-sentinel","modelType":"note","noteCard":{"displayTitle":"杭电选修"}}]}}`)
		case "/api/v1/feeds/detail":
			fmt.Fprint(w, `{"success":true,"data":{"data":{"note":{"noteId":"`+noteID+`","title":"杭电选修","desc":"考核交论文","user":{"userId":"author-private-sentinel"}},"comments":{"list":[],"hasMore":false}}}}`)
		default:
			t.Error("unexpected source operation")
			w.WriteHeader(404)
		}
	}))
	defer source.Close()
	rounds := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, _ := io.ReadAll(r.Body)
		if strings.Contains(string(request), "private-sentinel") {
			t.Error("source credentials or author reached model")
		}
		var delta any
		finish := "tool_calls"
		switch rounds {
		case 0:
			if !strings.Contains(string(request), "小红书（xiaohongshu）") {
				t.Error("configured source missing from agent prompt")
			}
			delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "search", "type": "function", "function": map[string]string{"name": "search_courses", "arguments": `{"query":"选修","source":"xiaohongshu"}`}}}}
		case 1:
			delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "read", "type": "function", "function": map[string]string{"name": "read_course_posts", "arguments": `{"posts":["post-1"]}`}}}}
		case 2:
			delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "show", "type": "function", "function": map[string]string{"name": "show_course_results", "arguments": `{}`}}}}
		default:
			delta, finish = map[string]string{"content": "这门课有同学提到交论文。[原帖](post-1)"}, "stop"
		}
		rounds++
		frame, _ := json.Marshal(map[string]any{"model": "deepseek-flash", "choices": []any{map[string]any{"delta": delta, "finish_reason": finish}}})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n", frame)
	}))
	defer model.Close()
	engine := Engine{Model: config.Model{BaseURL: model.URL, APIKey: "model-key", Name: "deepseek-flash"}, Client: tools.NewClient(t.TempDir()), Sources: config.Sources{Xiaohongshu: config.Xiaohongshu{Enabled: true, BaseURL: source.URL, AuthToken: "auth-private-sentinel"}}}
	result, err := engine.Answer(context.Background(), []storage.Message{{Role: "user", State: "complete", Content: "小红书有什么杭电选修体验？"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rounds != 4 || reads != 3 || result.Searches != 1 || result.Reads != 1 || result.CampusCalls != 1 || !strings.Contains(result.Text, "https://www.xiaohongshu.com/explore/"+noteID) || !strings.Contains(result.Text, "交论文") || strings.Contains(result.Text, "尚未取得") || strings.Contains(result.Text, "private-sentinel") {
		t.Fatal("agent source search/read/citation pipeline incomplete")
	}
}

func TestFavoriteStatusIsReturnedToModelWithoutAnswerOverride(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if fail && calls > 1 {
					w.WriteHeader(500)
					return
				}
				if calls > 1 {
					payload, _ := io.ReadAll(r.Body)
					if !strings.Contains(string(payload), "not_written") {
						t.Error("missing structured rejection")
					}
				}
				delta := map[string]any{"content": "请先提供可核实的课程。"}
				reason := "stop"
				if calls == 1 {
					delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "fav-test", "type": "function", "function": map[string]any{"name": "manage_course_collection", "arguments": `{"action":"add","classIDs":["invented"]}`}}}}
					reason = "tool_calls"
				}
				data, _ := json.Marshal(map[string]any{"model": "test", "choices": []any{map[string]any{"delta": delta, "finish_reason": reason}}})
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n", data)
			}))
			defer server.Close()
			engine := Engine{Model: config.Model{BaseURL: server.URL, APIKey: "test", Name: "test"}}
			var streamed strings.Builder
			result, err := engine.Answer(context.Background(), []storage.Message{{Role: "user", State: "complete", Content: "收藏测试课程"}}, func(e Event) {
				if e.Kind == "delta" {
					streamed.WriteString(e.Text)
				}
			})
			if (err != nil) != fail {
				t.Fatalf("unexpected error: %v", err)
			}
			if !fail && (result.Text != "请先提供可核实的课程。" || !strings.Contains(streamed.String(), result.Text)) {
				t.Fatal("model answer was suppressed or replaced")
			}
			if fail && result.Text != "" {
				t.Fatal("injected intermediate receipt on model failure")
			}

		})
	}
}
