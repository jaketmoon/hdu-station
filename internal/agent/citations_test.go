package agent

import (
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
