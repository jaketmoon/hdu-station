package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/jaketmoon/hdu-station/internal/storage"
	"github.com/jaketmoon/hdu-station/internal/tools"
)

// Explicit opt-in: this calls the configured paid model and the real QQ account.
// Reports contain only the visible answer and public source links, never raw tool data.
func TestLiveAcceptance(t *testing.T) {
	if os.Getenv("HDU_STATION_LIVE_TEST") != "1" {
		t.Skip("set HDU_STATION_LIVE_TEST=1 for the three real course questions")
	}
	root, err := config.Root()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	client := tools.NewClient(root)
	if state := client.Status(context.Background()); state != "ready" {
		t.Fatalf("QQ status: %s", state)
	}
	engine := &Engine{Model: cfg.Model, Client: client}
	questions := []string{
		"通识选修有什么水课？想找作业少、考核轻松的。",
		"有哪些给分比较高的通识选修？最好不用闭卷考试。",
		"人文经典类通识选修有哪些比较轻松的课？老师和考核有什么要注意的？",
	}
	report := fmt.Sprintf("# 真实选课验收\n\n时间：%s\n\n配置模型：%s\n\n", time.Now().Format(time.RFC3339), cfg.Model.Name)
	for i, question := range questions {
		t.Run(fmt.Sprintf("question_%d", i+1), func(t *testing.T) {
			start := time.Now()
			events := 0
			result, err := engine.Answer(context.Background(), []storage.Message{{Role: "user", Content: question, State: "complete"}}, func(e Event) {
				if e.Kind == "delta" {
					events++
				}
				if e.Kind == "status" {
					t.Log(e.Text)
				}
			})
			elapsed := time.Since(start).Round(time.Millisecond)
			report += fmt.Sprintf("## 问题 %d：%s\n\n耗时：%s；实际模型：%s；搜索调用：%d；成功读帖：%d；文本事件：%d。\n\n%s\n\n", i+1, question, elapsed, result.Model, result.Searches, result.Reads, events, result.Text)
			if err != nil {
				report += "失败：" + err.Error() + "\n\n"
				t.Error(err)
				return
			}
			if result.Model != "deepseek-flash" && result.Model != "deepseek-v4.1-flash" {
				t.Errorf("unexpected actual model: %s", result.Model)
			}
			if result.Searches == 0 || result.Reads == 0 || events == 0 {
				t.Error("real search, post read and streaming are required")
			}
			if len([]rune(result.Text)) < 160 {
				t.Error("answer is too short to be useful")
			}
			sources := map[string]bool{}
			for _, s := range result.Sources {
				if s.URL != "" {
					sources[s.URL] = true
				}
			}
			links := regexp.MustCompile(`https://pd\.qq\.com/s/[A-Za-z0-9]+`).FindAllString(result.Text, -1)
			if len(links) == 0 {
				t.Error("answer must cite a real post")
			}
			for _, link := range links {
				if !sources[link] {
					t.Error("answer cited a source that was not read")
				}
			}
			if strings.Contains(result.Text, cfg.Model.APIKey) || strings.Contains(result.Text, "post-1") {
				t.Error("answer exposed internal data")
			}
			report += "自动检查："
			if t.Failed() {
				report += "未通过\n\n"
			} else {
				report += "通过（另需人工核对建议与来源）\n\n"
			}
			t.Logf("completed in %s; model=%s searches=%d reads=%d citations=%d", elapsed, result.Model, result.Searches, result.Reads, len(links))
		})
		dir := filepath.Join(root, "logs")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "acceptance.md"), []byte(report), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
