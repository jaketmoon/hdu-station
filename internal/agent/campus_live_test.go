package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/jaketmoon/hdu-station/internal/storage"
	"github.com/jaketmoon/hdu-station/internal/tools"
)

type liveCampusCredential string

func (c liveCampusCredential) AccessTokenFor(context.Context, string) (string, error) {
	return string(c), nil
}

func TestLiveCampusMatchingAndFitThroughModel(t *testing.T) {
	if os.Getenv("HDU_STATION_LIVE_CAMPUS_MODEL") != "1" {
		t.Skip("explicit real-model and campus read acceptance only")
	}
	pat := os.Getenv("HDU_STATION_CAMPUS_TEST_TOKEN")
	if pat == "" {
		t.Fatal("temporary test credential required")
	}
	root, err := config.Root()
	if err != nil {
		t.Fatal("data root unavailable")
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal("model configuration unavailable")
	}
	engine := Engine{Model: cfg.Model, Campus: tools.NewCampusClient(liveCampusCredential(pat)), Sources: config.Sources{QQ: config.QQ{Disabled: true}}}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	result, err := engine.Answer(ctx, []storage.Message{{Role: "user", State: "complete", Content: "我在网上看到两门推荐：影视音乐鉴赏、戏曲鉴赏。请核实本学期开课；名字不完全一致就按完整名称、影视音乐这个核心词两步查找，选最接近的官方候选（相似的可以保留多个），再结合我的课表筛选能塞进空闲位置的班级。这轮不用搜索社区，也不用评价课程好坏。请给出课程号、老师、上课时间和是否能放进课表，不展示班级号；不要展示我的已选课表。"}}, nil)
	if err != nil {
		t.Fatal("live model flow failed; details withheld")
	}
	if strings.Contains(result.Text, pat) || strings.Contains(result.Text, cfg.Model.APIKey) {
		t.Fatal("credential reached visible result")
	}
	if os.MkdirAll(filepath.Join(root, "logs"), 0700) != nil {
		t.Fatal("report directory unavailable")
	}
	if os.WriteFile(filepath.Join(root, "logs", "campus-acceptance.md"), []byte(result.Text), 0600) != nil {
		t.Fatal("report could not be saved")
	}
	t.Logf("campus tool calls=%d; visible answer saved to data-root logs/campus-acceptance.md", result.CampusCalls)
	if result.CampusCalls < 2 || result.Searches != 0 || len(result.Text) < 100 {
		t.Fatal("model did not complete the campus matching and fit flow")
	}
}
