package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Turns    []string `json:"turns"`
	Expected string   `json:"expected"`
	Mode     string   `json:"mode"`
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
	root, err := config.Root()
	if err != nil {
		t.Fatal("data root unavailable")
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal("configuration unavailable")
	}
	campus, err := campusauth.New(root, cfg.CampusKey)
	if err != nil {
		t.Fatal("campus connection unavailable")
	}
	data, err := os.ReadFile("testdata/student_questions.json")
	if err != nil {
		t.Fatal(err)
	}
	var dataset struct {
		Cases []studentCase `json:"cases"`
	}
	if json.Unmarshal(data, &dataset) != nil {
		t.Fatal("invalid dataset")
	}
	dir := filepath.Join(root, "logs", "robustness-20260913")
	if os.MkdirAll(dir, 0700) != nil {
		t.Fatal("cannot create report directory")
	}
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
		if selected == "" {
			if c.Mode != "live_engine" || done[c.ID] {
				continue
			}
		} else if !strings.Contains(","+selected+",", ","+c.ID+",") {
			continue
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
				if strings.Contains(r.Text, "本次尚未完成具体课程") {
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
				ID       string              `json:"id"`
				Category string              `json:"category"`
				Expected string              `json:"expected"`
				At       string              `json:"at"`
				Mode     string              `json:"mode"`
				Turns    []studentTurnResult `json:"turns"`
			}{c.ID, c.Category, c.Expected, time.Now().Format(time.RFC3339), "live_engine_2_workers", turns}
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
			fmt.Printf("QA %s completed turns=%d seconds=%.1f flags=%v\n", c.ID, len(turns), seconds, flags)
		}(c)
	}
	wg.Wait()
}
