package agent

import (
	"github.com/jaketmoon/hdu-station/internal/storage"
	"github.com/jaketmoon/hdu-station/internal/tools"
	"regexp"
	"sort"
	"strings"
)

var postReference = regexp.MustCompile(`\bpost-[0-9]+\b`)
var incompleteReference = regexp.MustCompile(`(?:\b(?:p|po|pos|post|post-|post-[0-9]+)|https?://[^\s\])>，。；！？、]*)$`)
var referenceLink = regexp.MustCompile(`\[([^\]\n]+)\]\((post-[0-9]+)\)`)
var qqLink = regexp.MustCompile(`https://pd\.qq\.com/s/[A-Za-z0-9]+`)
var markdownQQLink = regexp.MustCompile(`\[([^\]\n]+)\]\((https://pd\.qq\.com/s/[A-Za-z0-9]+)\)`)

func previousSources(history []storage.Message) []tools.Post {
	sources := []tools.Post{}
	seen := map[string]bool{}
	for _, message := range history {
		if message.Role != "assistant" || message.State != "complete" {
			continue
		}
		for _, link := range qqLink.FindAllString(message.Content, -1) {
			if !seen[link] {
				sources = append(sources, tools.Post{URL: link})
				seen[link] = true
			}
		}
	}
	return sources
}

// Resolve the model's post notation against this turn's reads without imposing
// an answer format or passing connector identifiers to the interface.
func resolveCitations(text string, sources []tools.Post) string {
	links := make(map[string]string, len(sources))
	verified := make(map[string]bool, len(sources))
	for _, source := range sources {
		links[source.ID] = source.URL
		if source.URL != "" {
			verified[source.URL] = true
		}
	}
	text = referenceLink.ReplaceAllStringFunc(text, func(link string) string {
		parts := referenceLink.FindStringSubmatch(link)
		if url := links[parts[2]]; url != "" {
			return "[" + parts[1] + "](" + url + ")"
		}
		return parts[1] + "（原帖未核实）"
	})
	text = markdownQQLink.ReplaceAllStringFunc(text, func(link string) string {
		parts := markdownQQLink.FindStringSubmatch(link)
		label := postReference.ReplaceAllString(parts[1], "原帖")
		if verified[parts[2]] {
			return "[" + label + "](" + parts[2] + ")"
		}
		return label + "（原帖未核实）"
	})
	text = postReference.ReplaceAllStringFunc(text, func(id string) string {
		if link := links[id]; link != "" {
			return "[原帖](" + link + ")"
		}
		return "来源未核实"
	})
	return qqLink.ReplaceAllStringFunc(text, func(link string) string {
		if verified[link] {
			return link
		}
		return "（原帖未核实）"
	})
}

// Keep sources accessible even if a free-form answer omits citations entirely.
// This lists what was read, rather than claiming that every post supports every sentence.
func withSourceLinks(text string, sources []tools.Post) string {
	for _, source := range sources {
		if source.URL != "" && strings.Contains(text, source.URL) {
			return text
		}
	}
	ordered := append([]tools.Post(nil), sources...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	links := []string{}
	seen := map[string]bool{}
	escape := strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]")
	for _, source := range ordered {
		if source.URL == "" || seen[source.URL] {
			continue
		}
		seen[source.URL] = true
		title := []rune(strings.Join(strings.Fields(source.Title), " "))
		if len(title) > 24 {
			title = append(title[:24], '…')
		}
		label := escape.Replace(string(title))
		if label == "" {
			label = "原帖"
		}
		links = append(links, "["+label+"]("+source.URL+")")
	}
	if len(links) == 0 {
		return text
	}
	return text + "\n\n---\n\n本次阅读的讨论：\n\n" + strings.Join(links, " · ")
}

type citationStream struct {
	raw, visible string
	sources      func() []tools.Post
	emit         func(Event)
}

func (s *citationStream) consume(event Event) {
	switch event.Kind {
	case "reset":
		s.raw, s.visible = "", ""
		s.emit(event)
	case "delta":
		s.raw += event.Text
		s.publish(resolveCitations(incompleteReference.ReplaceAllString(s.raw, ""), s.sources()))
	default:
		s.emit(event)
	}
}
func (s *citationStream) publish(text string) {
	if text == s.visible {
		return
	}
	if strings.HasPrefix(text, s.visible) {
		s.emit(Event{Kind: "delta", Text: strings.TrimPrefix(text, s.visible)})
	} else {
		s.emit(Event{Kind: "reset"})
		s.emit(Event{Kind: "delta", Text: text})
	}
	s.visible = text
}
