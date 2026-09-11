package agent

import (
	"github.com/jaketmoon/hdu-station/internal/storage"
	"github.com/jaketmoon/hdu-station/internal/tools"
	"strings"
	"testing"
)

func TestFragmentedReferencesBecomeReadSourceLinks(t *testing.T) {
	visible := ""
	sources := []tools.Post{{ID: "post-12", URL: "https://pd.qq.com/s/source"}}
	stream := &citationStream{sources: func() []tools.Post { return sources }, emit: func(e Event) {
		switch e.Kind {
		case "reset":
			visible = ""
		case "delta":
			visible += e.Text
		}
		if strings.Contains(visible, "post-") {
			t.Fatal("internal reference flashed in stream")
		}
	}}
	for _, text := range []string{"推荐课（po", "st-1", "2", "），其他观点见 post-99。"} {
		stream.consume(Event{Kind: "delta", Text: text})
	}
	stream.publish(resolveCitations(stream.raw, sources))
	if visible != "推荐课（[原帖](https://pd.qq.com/s/source)），其他观点见 来源未核实。" {
		t.Fatalf("citation result = %q", visible)
	}
}

func TestMultiSourceCitationsAndHistoryExcludeSignatures(t *testing.T) {
	url := "https://www.xiaohongshu.com/explore/66abcdef1234567890abcdef"
	sources := []tools.Post{
		{ID: "post-1", Source: "xiaohongshu", URL: url},
		{ID: "post-2", Source: "zanao", Title: "课程讨论", Locator: "赞哦帖子 123（学校：test）"},
	}
	text := resolveCitations("[小红书](post-1)，[赞哦](post-2)，[编造](https://www.xiaohongshu.com/explore/66abcdef1234567890aaaaaa)。", sources)
	if !strings.Contains(text, "[小红书]("+url+")") || !strings.Contains(text, sources[1].Locator) || strings.Contains(text, "66abcdef1234567890aaaaaa") || strings.Contains(text, "post-") {
		t.Fatal("mixed source citations were not resolved")
	}
	if got := withSourceLinks("读到一些讨论。", sources[1:]); !strings.Contains(got, sources[1].Locator) || strings.Contains(got, "https://") {
		t.Fatal("Zanao footer invented a permalink or lost the locator")
	}
	previous := previousSources([]storage.Message{{Role: "assistant", State: "complete", Content: "[来源](" + url + ")"}})
	if len(previous) != 1 || !strings.Contains(resolveCitations(url, previous), url) {
		t.Fatal("Xiaohongshu history lost verified source")
	}
	signed := url + "?xsec_token=private-sentinel"
	if strings.Contains(resolveCitations("[签名]("+signed+") "+signed, sources), "private-sentinel") {
		t.Fatal("unverified signed URL remained in answer")
	}
	if len(previousSources([]storage.Message{{Role: "assistant", State: "complete", Content: signed}})) != 0 {
		t.Fatal("history accepted credentials in links")
	}
}

func TestFreeFormAnswerStillHasReadSources(t *testing.T) {
	sources := []tools.Post{{ID: "post-1", Title: "课程体验", URL: "https://pd.qq.com/s/source"}}
	text := withSourceLinks("可以考虑这门课。", sources)
	if !strings.HasPrefix(text, "可以考虑这门课。") || !strings.Contains(text, "[课程体验](https://pd.qq.com/s/source)") {
		t.Fatal("free-form response lost its sources")
	}
	if withSourceLinks(text, sources) != text {
		t.Fatal("duplicated source footer")
	}
}

func TestUnverifiedLinksCannotBecomeClickableSources(t *testing.T) {
	sources := []tools.Post{{ID: "post-1", URL: "https://pd.qq.com/s/real"}}
	text := resolveCitations("[详情](post-1)；[编造](https://pd.qq.com/s/fiction)；https://pd.qq.com/s/unknown", sources)
	if !strings.Contains(text, "[详情](https://pd.qq.com/s/real)") || strings.Contains(text, "/fiction") || strings.Contains(text, "/unknown") {
		t.Fatal("unverified citation remained clickable")
	}
}

func TestPostIDsUsedAsLinkLabelsAreNotNested(t *testing.T) {
	sources := []tools.Post{{ID: "post-12", URL: "https://pd.qq.com/s/source"}}
	text := resolveCitations("链接：[post-12](https://pd.qq.com/s/source)", sources)
	if text != "链接：[原帖](https://pd.qq.com/s/source)" {
		t.Fatalf("nested Markdown link: %s", text)
	}
}
