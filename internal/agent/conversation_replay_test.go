package agent

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/jaketmoon/hdu-station/internal/storage"
	"github.com/jaketmoon/hdu-station/internal/tools"
)

type replayGrant struct{}

func (replayGrant) AccessTokenFor(context.Context, string) (string, error) {
	return "fixture-only", nil
}

type replayClass struct {
	ClassID, CourseID, CourseName, TeacherName, ClassTime string
	Credit                                                int
	CampusID, ExaminationMethod                           string
}
type replayCase struct {
	ID, Question, Recommendation string
	Turns                        []string
}
type replayData struct {
	Cases   []replayCase
	Classes []replayClass
}

// Real model, original recommendation and user turns, deterministic catalog and
// isolated favorite endpoint. No request may write the user's real collection.
func TestLiveConversationReplays(t *testing.T) {
	if os.Getenv("HDU_STATION_LIVE_REPLAYS") != "1" {
		t.Skip("explicit real-model replay")
	}
	root, err := config.Root()
	if err != nil {
		t.Fatal("data root unavailable")
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal("configuration unavailable")
	}
	data, err := os.ReadFile("testdata/conversation_replays.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture replayData
	if json.Unmarshal(data, &fixture) != nil {
		t.Fatal("invalid replay fixture")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.ID, func(t *testing.T) { runConversationReplay(t, cfg.Model, tc, fixture.Classes, root) })
	}
}

func runConversationReplay(t *testing.T, model config.Model, tc replayCase, classes []replayClass, root string) {
	var mu sync.Mutex
	stored := []string{"existing-original"}
	searches := []string{}
	writes := 0
	campus := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/hduhelp-neo/academic/config":
			fmt.Fprint(w, `{"code":0,"data":{"courseQueryDefault":{"schoolYear":"2026-2027","semester":1},"scheduleDefault":{"schoolYear":"2026-2027","semester":1}}}`)
		case "/hduhelp-neo/academic/class/map":
			fmt.Fprint(w, `{"code":0,"data":{"campusID":{"1":"下沙"},"examinationMethod":{"2":"考查"}}}`)
		case "/hduhelp-neo/academic/class/search":
			query := r.URL.Query().Get("query")
			searches = append(searches, query)
			rows := []replayClass{}
			for _, o := range classes {
				matches := true
				for _, word := range strings.Fields(query) {
					matches = matches && strings.Contains(o.CourseName, word)
				}
				if matches {
					rows = append(rows, o)
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"classes": rows}})
		case "/hduhelp-neo/academic/class/fav":
			if r.Method == "POST" {
				var body struct{ Classes []string }
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Error("invalid write")
				}
				stored = body.Classes
				writes++
				fmt.Fprint(w, `{"code":0}`)
				return
			}
			rows := []map[string]string{}
			for _, id := range stored {
				rows = append(rows, map[string]string{"classID": id})
			}
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": rows})
		case "/hduhelp-neo/academic/course":
			rows := []replayClass{}
			for _, o := range classes {
				for _, id := range r.URL.Query()["id"] {
					if o.ClassID == id {
						rows = append(rows, o)
					}
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"classes": rows}})
		default:
			t.Errorf("unexpected campus route %s", r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	defer campus.Close()
	// All fixed campus HTTPS routes terminate at the fixture; model HTTPS keeps
	// certificate verification and its normal destination. No global parallelism.
	old := http.DefaultTransport
	transport := old.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialTLSContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address == "api.hduhelp.com:443" {
			return (&tls.Dialer{Config: campus.Client().Transport.(*http.Transport).TLSClientConfig}).DialContext(ctx, network, campus.Listener.Addr().String())
		}
		return (&tls.Dialer{}).DialContext(ctx, network, address)
	}
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = old; transport.CloseIdleConnections() }()
	var contexts tools.CourseContexts
	engine := Engine{Model: model, Contexts: &contexts, Campus: tools.NewCampusClient(replayGrant{}), Client: tools.NewClient(t.TempDir()), Sources: config.Sources{QQ: config.QQ{Disabled: true}}}
	history := []storage.Message{{ConversationID: tc.ID, Role: "user", State: "complete", Content: tc.Question}, {ConversationID: tc.ID, Role: "assistant", State: "complete", Content: tc.Recommendation}}
	expected := map[string]bool{"影视音乐赏析": true, "中国民族器乐鉴赏": true, "知识产权案例精讲": true, "中国电视发展史": true, "中国建筑设计赏析": true, "世界政治经济与国际关系": true}
	if tc.ID == "replay-2" {
		expected["电影世界中的法律文化"] = true
		expected["法律与中国社会问题"] = true
	}
	reports := []map[string]any{}
	for i, q := range tc.Turns {
		history = append(history, storage.Message{ConversationID: tc.ID, Role: "user", State: "complete", Content: q})
		before := len(searches)
		r, err := engine.Answer(context.Background(), history, nil)
		history = append(history, storage.Message{ConversationID: tc.ID, Role: "assistant", State: "complete", Content: r.Text})
		reports = append(reports, map[string]any{"question": q, "answer": r.Text, "actions": r.Actions, "searches": append([]string{}, searches[before:]...), "stored": append([]string{}, stored...), "writes": writes})
		t.Logf("turn=%d actions=%v added=%d searches=%d", i+1, r.Actions, len(stored)-1, len(searches)-before)
		if err != nil {
			t.Errorf("replay failed: %v", err)
			break
		}
		if i == 0 && writes == 0 {
			t.Error("explicit collection request stopped without writing")
		}
		if i == 0 {
			present := map[string]bool{}
			for _, id := range stored {
				present[id] = true
			}
			for _, o := range classes {
				if expected[o.CourseName] && !present[o.ClassID] {
					t.Errorf("initial request left class uncollected: %s %s", o.CourseName, o.ClassTime)
				}
			}
			required := []string{"影视音乐", "中国民族器乐", "图画书", "中国电视", "中国建筑", "化学品", "知识产权", "舞蹈", "珠宝", "世界政治"}
			if tc.ID == "replay-1" {
				required = append(required, "名人生命", "大学生与法")
			} else {
				required = append(required, "电影世界", "法律与中国社会", "美国文化", "数字中国", "法律心理", "道德经")
			}
			// Scripted plumbing regression only queries fixture courses; the
			// real-model replay also checks unavailable recommended courses.
			if model.Name != "test" {
				for _, name := range required {
					found := false
					for _, q := range searches {
						found = found || strings.Contains(q, name)
					}
					if !found {
						t.Errorf("recommended course never checked: %s", name)
					}
				}
			}
		}
		if i > 0 && len(searches) != before {
			t.Error("follow-up unnecessarily searched again")
		}
		for _, bad := range []string{"来源：校园教务查询。仅展示", "修改前读取的课程收藏", "只能选一个班", "请你告诉我按哪个规则加", "你说一声", "你确认后", "你点头"} {
			if strings.Contains(r.Text, bad) {
				t.Errorf("obsolete behavior: %s", bad)
			}
		}
	}
	has := map[string]bool{}
	for _, id := range stored {
		has[id] = true
	}
	if !has["existing-original"] {
		t.Error("lost original favorite")
	}
	for _, o := range classes {
		if expected[o.CourseName] && !has[o.ClassID] {
			t.Errorf("missing requested class: %s %s", o.CourseName, o.ClassTime)
		}
		if strings.HasPrefix(o.CourseName, "人工智能") && has[o.ClassID] {
			t.Errorf("unrelated course added: %s", o.CourseName)
		}
	}
	dir := filepath.Join(root, "logs", "conversation-replays-20260917")
	os.MkdirAll(dir, 0700)
	report, _ := json.MarshalIndent(reports, "", "  ")
	os.WriteFile(filepath.Join(dir, tc.ID+".json"), report, 0600)
}

// Offline regression: a valid display must neither stop subsequent writes nor
// replace the model's answer. A follow-up reuses host evidence, not prose IDs.
func TestQueryDisplayWriteAndCrossTurnReuse(t *testing.T) {
	raw, err := os.ReadFile("testdata/conversation_replays.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture replayData
	json.Unmarshal(raw, &fixture)
	wanted := map[string]bool{"影视音乐赏析": true, "中国民族器乐鉴赏": true, "知识产权案例精讲": true, "中国电视发展史": true, "中国建筑设计赏析": true, "电影世界中的法律文化": true, "法律与中国社会问题": true, "世界政治经济与国际关系": true}
	ids := []string{}
	names := []string{}
	seen := map[string]bool{}
	for _, o := range fixture.Classes {
		if wanted[o.CourseName] {
			ids = append(ids, o.ClassID)
			if !seen[o.CourseName] {
				names = append(names, o.CourseName)
				seen[o.CourseName] = true
			}
		}
	}
	rounds := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rounds++
		name := ""
		args := any(nil)
		switch rounds {
		case 1:
			name = "check_course_offerings"
			args = map[string]any{"courses": names}
		case 2:
			name = "show_course_results"
			args = map[string]any{}
		case 3, 5, 7:
			name = "manage_course_collection"
			args = map[string]any{"action": "add", "classIDs": ids}
		}
		delta := map[string]any{"content": "已收藏8门课程，共22个教学班。"}
		reason := "stop"
		if name != "" {
			encoded, _ := json.Marshal(args)
			delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprint(rounds), "type": "function", "function": map[string]any{"name": name, "arguments": string(encoded)}}}}
			reason = "tool_calls"
		}
		frame, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": reason}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n", frame)
	}))
	defer model.Close()
	runConversationReplay(t, config.Model{BaseURL: model.URL, APIKey: "test", Name: "test"}, fixture.Cases[1], fixture.Classes, t.TempDir())
	if rounds != 8 {
		t.Fatalf("model continuation was cut short: %d", rounds)
	}
}
