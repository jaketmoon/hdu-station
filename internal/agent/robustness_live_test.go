package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jaketmoon/hdu-station/internal/campusauth"
	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/jaketmoon/hdu-station/internal/storage"
	"github.com/jaketmoon/hdu-station/internal/tools"
)

type studentCase struct {
	ID               string   `json:"id"`
	Category         string   `json:"category"`
	Turns            []string `json:"turns"`
	Expected         string   `json:"expected"`
	OriginalExpected string   `json:"originalExpected"`
	Support          string   `json:"support"`
	Mode             string   `json:"mode"`
}

var studentQARunPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func studentQARunDirectory(root, run string) (string, error) {
	if !studentQARunPattern.MatchString(run) {
		return "", fmt.Errorf("HDU_STATION_QA_RUN must be 1–64 ASCII letters, digits, hyphens or underscores, starting with a letter or digit")
	}
	return filepath.Join(root, "logs", "robustness-iterations", run), nil
}

func studentQASelected(c studentCase, selected string, done map[string]bool) bool {
	if selected == "all" {
		return !done[c.ID]
	}
	if selected == "" {
		return c.Mode == "live_engine" && !done[c.ID]
	}
	for _, id := range strings.Split(selected, ",") {
		if strings.TrimSpace(id) == c.ID {
			return true // Explicit IDs record another attempt in the same run.
		}
	}
	return false
}

type studentTurnResult struct {
	Question    string            `json:"question"`
	Answer      string            `json:"answer"`
	Error       string            `json:"error,omitempty"`
	Seconds     float64           `json:"seconds"`
	Searches    int               `json:"searches"`
	Reads       int               `json:"reads"`
	CampusCalls int               `json:"campus_calls"`
	Connections map[string]string `json:"connections"`
	Statuses    []string          `json:"statuses"`
	Flags       []string          `json:"flags"`
}

// Runs the production Engine against the saved Station connections. It writes
// only visible answers and safe progress messages, never tool payloads or keys.
// The flags detect mechanical failures; semantic acceptance requires review.
func TestLiveStudentQuestionDataset(t *testing.T) {
	if os.Getenv("HDU_STATION_LIVE_QA") != "1" {
		t.Skip("explicit real-model questionnaire acceptance only")
	}
	run := os.Getenv("HDU_STATION_QA_RUN")
	if run == "" {
		run = "run-" + strings.ReplaceAll(time.Now().UTC().Format("20060102-150405.000000000"), ".", "-")
	}
	root, err := config.Root()
	if err != nil {
		t.Fatal("data root unavailable")
	}
	dir, err := studentQARunDirectory(root, run)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal("configuration unavailable")
	}
	campus, err := campusauth.New(root, cfg.CampusKey)
	if err != nil {
		t.Fatal("campus connection unavailable")
	}
	datasetPath := "testdata/student_questions.json"
	if os.Getenv("HDU_STATION_QA_SCOPED") == "1" {
		datasetPath = "testdata/student_questions_scoped.json"
	}
	data, err := os.ReadFile(datasetPath)
	if err != nil {
		t.Fatal(err)
	}
	var dataset struct {
		Cases []studentCase `json:"cases"`
	}
	if json.Unmarshal(data, &dataset) != nil {
		t.Fatal("invalid dataset")
	}
	if os.MkdirAll(dir, 0700) != nil {
		t.Fatal("cannot create report directory")
	}
	// Keep the exact expectations with each run. Never merge changed acceptance
	// criteria or a scoped dataset into a prior run's results.
	snapshot := filepath.Join(dir, "questions.json")
	if old, e := os.ReadFile(snapshot); e == nil {
		if string(old) != string(data) {
			t.Fatal("dataset changed: choose a new HDU_STATION_QA_RUN")
		}
	} else if !os.IsNotExist(e) || os.WriteFile(snapshot, data, 0600) != nil {
		t.Fatal("cannot save dataset snapshot")
	}
	t.Logf("QA run=%s dataset=%s output=%s", run, datasetPath, dir)
	out := filepath.Join(dir, "live-results.jsonl")
	selected := os.Getenv("HDU_STATION_QA_CASES")
	done := map[string]bool{}
	if f, e := os.Open(out); e == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 4096), 4<<20)
		for sc.Scan() {
			var r struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				done[r.ID] = true
			}
		}
		if sc.Err() != nil {
			f.Close()
			t.Fatal("cannot read previous QA results")
		}
		f.Close()
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("cannot open report")
	}
	defer f.Close()
	client := tools.NewClient(root)
	engine := Engine{Model: cfg.Model, Client: client, Sources: cfg.Sources, Campus: tools.NewCampusClient(campus)}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, 2)
	for _, c := range dataset.Cases {
		if !studentQASelected(c, selected, done) {
			continue
		}
		if c.OriginalExpected == "" {
			c.OriginalExpected = c.Expected
		}
		if c.Support == "" {
			c.Support = "unscoped"
		}
		slots <- struct{}{}
		wg.Add(1)
		go func(c studentCase) {
			defer wg.Done()
			defer func() { <-slots }()
			history := []storage.Message{}
			turns := []studentTurnResult{}
			for _, q := range c.Turns {
				history = append(history, storage.Message{Role: "user", State: "complete", Content: q})
				start := time.Now()
				statuses := []string{}
				r, e := engine.Answer(context.Background(), history, func(ev Event) {
					if ev.Kind == "status" {
						statuses = append(statuses, ev.Text)
					}
				})
				row := studentTurnResult{Question: q, Answer: r.Text, Seconds: time.Since(start).Seconds(), Searches: r.Searches, Reads: r.Reads, CampusCalls: r.CampusCalls, Connections: r.Connections, Statuses: statuses, Flags: []string{}}
				if e != nil {
					row.Error = e.Error()
					row.Flags = append(row.Flags, "request_error")
				}
				if strings.TrimSpace(r.Text) == "" {
					row.Flags = append(row.Flags, "empty_answer")
				}
				for _, needle := range []string{"suggestedPlan", "candidateFits", "先从社区讨论获取具体候选课程", "再传课程名调用本工具"} {
					if strings.Contains(r.Text, needle) {
						row.Flags = append(row.Flags, "internal_tool_text")
						break
					}
				}
				if strings.Contains(r.Text, "本次尚未完成具体课程") || strings.Contains(r.Text, "这轮尚未完成具体课程") {
					row.Flags = append(row.Flags, "incomplete_course_check")
				}
				for _, secret := range []string{cfg.Model.APIKey, cfg.CampusKey, cfg.Sources.Xiaohongshu.AuthToken, cfg.Sources.Zanao.Token} {
					if secret != "" && strings.Contains(row.Answer, secret) {
						row.Answer = "[credential redacted]"
						row.Flags = append(row.Flags, "credential_leak")
					}
				}
				if e == nil {
					history = append(history, storage.Message{Role: "assistant", State: "complete", Content: row.Answer})
				}
				turns = append(turns, row)
			}
			record := struct {
				ID               string              `json:"id"`
				Category         string              `json:"category"`
				Expected         string              `json:"expected"`
				OriginalExpected string              `json:"originalExpected"`
				Support          string              `json:"support"`
				Run              string              `json:"run"`
				At               string              `json:"at"`
				Mode             string              `json:"mode"`
				Turns            []studentTurnResult `json:"turns"`
			}{c.ID, c.Category, c.Expected, c.OriginalExpected, c.Support, run, time.Now().Format(time.RFC3339), "live_engine_2_workers", turns}
			b, _ := json.Marshal(record)
			mu.Lock()
			defer mu.Unlock()
			if _, e := f.Write(append(b, '\n')); e != nil {
				t.Error("cannot save QA result")
			}
			f.Sync()
			flags := []string{}
			seconds := 0.0
			for _, r := range turns {
				flags = append(flags, r.Flags...)
				seconds += r.Seconds
			}
			fmt.Printf("QA %s support=%s completed turns=%d seconds=%.1f flags=%v\n", c.ID, c.Support, len(turns), seconds, flags)
		}(c)
	}
	wg.Wait()
}

func TestStudentQARunDirectory(t *testing.T) {
	root := t.TempDir()
	for _, invalid := range []string{"", ".", "..", "../baseline", "/tmp/run", "a/b", `a\b`, "a b", "测试", strings.Repeat("a", 65)} {
		if _, err := studentQARunDirectory(root, invalid); err == nil {
			t.Fatalf("unsafe run accepted: %q", invalid)
		}
	}
	got, err := studentQARunDirectory(root, "i01-source_fix")
	if err != nil || got != filepath.Join(root, "logs", "robustness-iterations", "i01-source_fix") {
		t.Fatalf("unexpected safe run directory: %q, %v", got, err)
	}
}

func TestStudentQASelection(t *testing.T) {
	r := studentCase{ID: "R01", Mode: "computer_use"}
	e := studentCase{ID: "E01", Mode: "live_engine"}
	done := map[string]bool{"R01": true}
	if studentQASelected(r, "", nil) || !studentQASelected(r, "all", nil) || studentQASelected(r, "all", done) || !studentQASelected(r, "R01, E01", done) || !studentQASelected(e, "R01, E01", nil) {
		t.Fatal("selection/resume semantics changed")
	}
}

func TestScopedStudentDatasetPreservesOriginalQuestions(t *testing.T) {
	read := func(path string) []studentCase {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var dataset struct {
			Cases []studentCase `json:"cases"`
		}
		if err := json.Unmarshal(data, &dataset); err != nil {
			t.Fatal(err)
		}
		return dataset.Cases
	}
	original := read("testdata/student_questions.json")
	scoped := read("testdata/student_questions_scoped.json")
	if len(original) != 60 || len(scoped) != len(original) {
		t.Fatal("all 60 original cases must remain in the scoped dataset")
	}
	turns, boundaries := 0, 0
	for i, c := range scoped {
		o := original[i]
		if c.ID != o.ID || c.Category != o.Category || c.Mode != o.Mode || !reflect.DeepEqual(c.Turns, o.Turns) || c.OriginalExpected != o.Expected || c.Expected == "" {
			t.Fatalf("original case/expectation lost at %s", c.ID)
		}
		if c.Support != "supported" && c.Support != "boundary" {
			t.Fatalf("missing scope classification at %s", c.ID)
		}
		turns += len(c.Turns)
		if c.Support == "boundary" {
			boundaries++
		}
	}
	if turns != 65 || boundaries == 0 || boundaries == len(scoped) {
		t.Fatal("unexpected turn count or missing honest support split")
	}
}
