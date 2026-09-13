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
	request := parseCourseRequest(history)
	if request.Boundary != "" {
		emit(Event{Kind: "delta", Text: request.Boundary})
		return Result{Text: request.Boundary, Model: e.Model.Name}, nil
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
	campus.XiashaOnly = true
	campusDisplay := ""
	verify, err := utils.InferTool("check_course_offerings", "一次核实本轮全部课程：当前学期下沙开课、完整名称后核心词检索、按用户要求检查本人课表和时间偏好，直接展示结果并结束。必须提供具体课程名称；不支持只读课表。", func(ctx context.Context, in tools.VerifyCoursesInput) (map[string]string, error) {
		if request.EnrolledCourse != "" {
			in.Courses = []string{request.EnrolledCourse}
		}
		display, err := campus.VerifyCourses(ctx, in, tools.VerifyOptions{MatchSchedule: request.MatchSchedule, AllowedDays: request.AllowedDays, AllowedSections: request.AllowedSections, MaxCourses: request.MaxCourses, MinCourses: request.MinCourses})
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return map[string]string{"status": "invalid_courses", "nextStep": "请提供1至12个具体课程名称"}, nil
		}
		if session.Reads > 0 {
			display = strings.TrimSpace(in.CommunitySummary) + "\n\n" + campus.CommunityCaveat(in.CommunitySummary) + display
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
		if !request.CommunityOnly && event.Kind == "delta" {
			return
		}
		citations.consume(event)
	})
	provider.requiredTool = func() string {
		if request.RequireVerification {
			return "check_course_offerings"
		}
		return ""
	}
	provider.completed = func() string { return campusDisplay }
	provider.completionGap = func() string {
		if campusDisplay == "" && !request.CommunityOnly {
			return "推荐尚未完成：从本轮帖子或对话中取全部具体课程，调用 check_course_offerings 核实后结束。若没有具体候选，明确说明未找到，不编造课程。"
		}
		return ""
	}
	provider.canUseTools = func() bool { return session.Calls+campus.Calls < 12 }
	availableTools := []tool.BaseTool{search, read, verify}
	if request.OfficialOnly {
		availableTools = []tool.BaseTool{verify}
		prompt += "\n本轮仅核实具体课程的教务信息，不搜索社区。直接调用 check_course_offerings；课程不明确时要求补充名称。"
	}
	runner, err := react.NewAgent(ctx, &react.AgentConfig{ToolCallingModel: provider, MaxStep: 26, ToolsConfig: withToolRecovery(compose.ToolsNodeConfig{Tools: availableTools, ExecuteSequentially: true})})
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
	} else if !request.CommunityOnly {
		text = "这轮尚未完成具体课程的本学期开课核实，不能确认推荐可选。请提供完整官方课程名或课程号后重试。"
		if session.Reads > 0 {
			text += "已读社区资料仅供参考，不能替代开课核实。"
		}
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
