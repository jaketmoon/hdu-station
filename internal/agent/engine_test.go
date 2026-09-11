package agent

import (
	"context"
	"fmt"
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
