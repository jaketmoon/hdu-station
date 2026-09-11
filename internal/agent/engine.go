package agent

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/jaketmoon/hdu-station/internal/skills"
	"github.com/jaketmoon/hdu-station/internal/storage"
	"github.com/jaketmoon/hdu-station/internal/tools"
)

type Result struct {
	Text            string
	Model           string
	Searches, Reads int
	Sources         []tools.Post
}
type Engine struct {
	Model  config.Model
	Client *tools.Client
}

func (e *Engine) Answer(ctx context.Context, history []storage.Message, emit func(Event)) (Result, error) {
	if emit == nil {
		emit = func(Event) {}
	}
	if strings.TrimSpace(e.Model.APIKey) == "" {
		return Result{}, errors.New("请先在设置中填写模型 API Key")
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	prompt, guilds := skills.CourseSelection()
	session := tools.NewSession(e.Client, guilds, func(text string) { emit(Event{Kind: "status", Text: text}) })
	search, err := utils.InferTool("search_courses", "搜索三个杭电频道中的课程讨论，返回标题和帖子 id。用简短关键词，选择相关帖子后读取正文与评论。", session.Search)
	if err != nil {
		return Result{}, err
	}
	read, err := utils.InferTool("read_course_posts", "读取搜索到的帖子正文、评论与原帖链接，一次最多六个帖子。", session.Read)
	if err != nil {
		return Result{}, err
	}
	previous := previousSources(history)
	citations := &citationStream{sources: func() []tools.Post { return append(session.Sources(), previous...) }, emit: emit}
	provider := NewProvider(e.Model, citations.consume)
	provider.canUseTools = func() bool { return session.Reads < 12 && session.Calls < 10 }
	runner, err := react.NewAgent(ctx, &react.AgentConfig{ToolCallingModel: provider, MaxStep: 22, ToolsConfig: compose.ToolsNodeConfig{Tools: []tool.BaseTool{search, read}, ExecuteSequentially: true}})
	if err != nil {
		return Result{}, errors.New("无法启动选课助手")
	}
	input := []*schema.Message{schema.SystemMessage(prompt + "\n今天是 " + time.Now().Format("2006-01-02") + "。")}
	// Keep recent visible messages only. Tool payloads never persist between turns.
	recent := []*schema.Message{}
	used := 0
	for i := len(history) - 1; i >= 0; i-- {
		m := history[i]
		if m.State != "complete" || m.Content == "" {
			continue
		}
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		if used+len(m.Content) > 32000 || len(recent) >= 12 {
			break
		}
		used += len(m.Content)
		recent = append(recent, &schema.Message{Role: schema.RoleType(m.Role), Content: m.Content})
	}
	for i := len(recent) - 1; i >= 0; i-- {
		input = append(input, recent[i])
	}
	message, err := runner.Generate(ctx, input)
	result := Result{Model: provider.state.actualModel, Searches: session.Searches, Reads: session.Reads, Sources: session.Sources()}
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, errors.New(safeError(err))
	}
	result.Text = withSourceLinks(resolveCitations(message.Content, citations.sources()), result.Sources)
	citations.publish(result.Text)
	return result, nil
}

// Eino adds node information to errors; expose only our stable actionable messages.
func safeError(err error) string {
	text := err.Error()
	for _, known := range []string{"模型 API Key 无效或没有访问权限，请检查设置", "模型请求限流，请稍后重试", "模型连接失败，请检查网络和设置", "模型连接中断，可以重新发送问题", "模型没有返回内容，请重试", "回答达到长度上限，请缩小问题范围后重试", "这轮对话内容过长，请开启新对话", "QQ 频道连接组件尚未安装，请在设置中安装"} {
		if strings.Contains(text, known) {
			return known
		}
	}
	return "这次回答未能完成，请稍后重试"
}
