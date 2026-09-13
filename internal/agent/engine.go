package agent

import (
	"context"
	"errors"
	"regexp"
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
	CampusCalls     int
	Sources         []tools.Post
	Connections     map[string]string
}
type Engine struct {
	Model   config.Model
	Client  *tools.Client
	Sources config.Sources
	Campus  *tools.CampusClient
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
	session := tools.NewSession(e.Client, guilds, func(text string) { emit(Event{Kind: "status", Text: text}) }, e.Sources)
	search, err := utils.InferTool("search_courses", "在已启用的 QQ 频道、赞哦、小红书中搜索课程讨论，返回来源、标题和帖子 id。可指定 source；选择相关帖子后读取正文与评论。", session.Search)
	if err != nil {
		return Result{}, err
	}
	read, err := utils.InferTool("read_course_posts", "读取搜索到的帖子正文、评论与原帖链接，一次最多六个帖子。", session.Read)
	if err != nil {
		return Result{}, err
	}
	campus := tools.NewCampusSession(e.Campus, func(text string) { emit(Event{Kind: "status", Text: text}) })
	offerings, err := utils.InferTool("check_course_offerings", "核实推荐课程的开课班级、老师、学分、时间与考核。先完整名称检索，没有精确匹配再查核心词；可用 coreKeywords 指定核心词。模糊候选 candidates 按课程号分组，模型选最接近者（相似时保留多个），将其 courseIDs 传入再次查询或课表筛选。未找到不等于未开课。", campus.CheckOfferings)
	if err != nil {
		return Result{}, err
	}
	fit, err := utils.InferTool("fit_courses_to_schedule", "读取本人完整课表，按全部周次、星期与节次判断候选班级能否放入空闲位置。提供课程名可直接检查开课和冲突；省略课程名只读取课表占用时段。遵守工具计算的 fits/conflict/unknown 等状态，suggestedPlan 才是彼此也无冲突的候选组合。", campus.FitCourses)
	if err != nil {
		return Result{}, err
	}
	campusDisplay := ""
	show, err := utils.InferTool("show_course_results", "完成校园核实后显示Go生成的课程、班级号、时间、冲突表并结束回答。用户要求按课表筛选时 matchSchedule=true；若尚未读课表会补查。模糊候选须先选定课程号。可附本轮社区经验摘要，不手写校园事实表。", func(ctx context.Context, in tools.ShowCoursesInput) (map[string]string, error) {
		display, err := campus.ShowCourses(ctx, in)
		if err != nil {
			return nil, err
		}
		if display == "" {
			return map[string]string{"status": "incomplete", "nextStep": campus.CompletionGap()}, nil
		}
		if session.Reads > 0 && strings.TrimSpace(in.CommunitySummary) != "" {
			display = strings.TrimSpace(in.CommunitySummary) + "\n\n" + display
		}
		campusDisplay = display
		return map[string]string{"display": display}, nil
	})
	if err != nil {
		return Result{}, err
	}
	previous := previousSources(history)
	citations := &citationStream{sources: func() []tools.Post { return append(session.Sources(), previous...) }, emit: emit}
	provider := NewProvider(e.Model, func(event Event) {
		// Official campus facts are rendered from the tool records, so a model
		// cannot briefly stream a mismatched class/time table before replacement.
		if campus.Calls > 0 && event.Kind == "delta" {
			return
		}
		citations.consume(event)
	})
	provider.requiredTool = campus.RequiredTool
	provider.completed = func() string { return campusDisplay }
	provider.completionGap = campus.CompletionGap
	provider.canUseTools = func() bool { return session.Calls+campus.Calls < 12 }
	runner, err := react.NewAgent(ctx, &react.AgentConfig{ToolCallingModel: provider, MaxStep: 26, ToolsConfig: compose.ToolsNodeConfig{Tools: []tool.BaseTool{search, read, offerings, fit, show}, ExecuteSequentially: true}})
	if err != nil {
		return Result{}, errors.New("无法启动选课助手")
	}
	input := []*schema.Message{schema.SystemMessage(prompt + "\n" + session.SourceSummary() + "\n今天是 " + time.Now().Format("2006-01-02") + "。")}
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
	result := Result{Model: provider.state.actualModel, Searches: session.Searches, Reads: session.Reads, CampusCalls: campus.Calls, Sources: session.Sources(), Connections: session.ConnectionStates()}
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, errors.New(safeError(err))
	}
	text := message.Content
	if campusDisplay != "" {
		text = campusDisplay
	} else if campus.Calls > 0 {
		text = campus.Display()
	}
	result.Text = withSourceLinks(resolveCitations(text, citations.sources()), result.Sources)
	citations.publish(result.Text)
	return result, nil
}

// Eino adds node information to errors; expose only our stable actionable messages.
func safeError(err error) string {
	text := err.Error()
	if strings.Contains(text, "failed to unmarshal arguments") {
		return "模型生成的查询参数格式不正确，请重试"
	}
	if strings.Contains(text, "not found in toolsNode indexes") {
		return "模型请求了不可用的查询工具，请重试"
	}
	if errors.Is(err, compose.ErrExceedMaxSteps) || strings.Contains(text, "exceeds max steps") {
		return "本轮查询步骤已达上限，请缩小问题范围后重试"
	}
	if status := regexp.MustCompile(`模型暂时不可用（HTTP ([0-9]{3})）`).FindString(text); status != "" {
		return status + "，请稍后重试"
	}
	for _, known := range []string{"模型生成中断，请重试", "模型返回了无法解析的响应", "模型工具请求不完整", "模型请求了过多工具", "工具请求超过长度限制", "回答超过长度限制，请缩小问题范围"} {
		if strings.Contains(text, known) {
			return known
		}
	}
	for _, known := range []string{"模型 API Key 无效或没有访问权限，请检查设置", "模型请求限流，请稍后重试", "模型连接失败，请检查网络和设置", "模型连接中断，可以重新发送问题", "模型没有返回内容，请重试", "回答达到长度上限，请缩小问题范围后重试", "这轮对话内容过长，请开启新对话", "QQ 频道连接组件尚未安装，请在设置中安装"} {
		if strings.Contains(text, known) {
			return known
		}
	}
	return "这次回答未能完成，请稍后重试"
}
