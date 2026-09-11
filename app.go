package main

import (
	"context"
	"errors"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jaketmoon/hdu-station/internal/agent"
	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/jaketmoon/hdu-station/internal/storage"
	"github.com/jaketmoon/hdu-station/internal/tools"
	wails "github.com/wailsapp/wails/v2/pkg/runtime"
)

type Settings struct {
	BaseURL   string `json:"baseURL"`
	Model     string `json:"model"`
	HasAPIKey bool   `json:"hasAPIKey"`
	QQStatus  string `json:"qqStatus"`
	DataRoot  string `json:"dataRoot"`
}
type SettingsInput struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey"`
}
type TurnEvent struct {
	RequestID      string                `json:"requestId"`
	ConversationID string                `json:"conversationId"`
	Kind           string                `json:"kind"`
	Text           string                `json:"text"`
	Conversation   *storage.Conversation `json:"conversation,omitempty"`
	User           *storage.Message      `json:"user,omitempty"`
	Assistant      *storage.Message      `json:"assistant,omitempty"`
}
type TurnResult struct {
	Conversation storage.Conversation `json:"conversation"`
	Message      storage.Message      `json:"message"`
	Error        string               `json:"error,omitempty"`
}
type activeTurn struct {
	id, conversationID string
	cancel             context.CancelFunc
	done               chan struct{}
}
type App struct {
	mu     sync.Mutex
	ctx    context.Context
	root   string
	cfg    config.Config
	store  *storage.Store
	client *tools.Client
	active *activeTurn
	emit   func(TurnEvent)
	answer func(context.Context, config.Model, []storage.Message, func(agent.Event)) (agent.Result, error)
}

func (a *App) OpenLink(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "pd.qq.com" || u.User != nil {
		return errors.New("只支持打开 QQ 频道的原帖链接")
	}
	wails.BrowserOpenURL(a.ctx, u.String())
	return nil
}

func (a *App) CopyText(text string) error {
	if len(text) > 128<<10 {
		return errors.New("复制内容过长")
	}
	return wails.ClipboardSetText(a.ctx, text)
}

func createApplication() (*App, error) {
	root, err := config.Root()
	if err != nil {
		return nil, err
	}
	return createApplicationAt(root)
}
func createApplicationAt(root string) (*App, error) {
	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}
	store, err := storage.Open(root)
	if err != nil {
		return nil, errors.New("无法打开本机对话记录")
	}
	a := &App{ctx: context.Background(), root: root, cfg: cfg, store: store, client: tools.NewClient(root), emit: func(TurnEvent) {}}
	a.answer = func(ctx context.Context, m config.Model, h []storage.Message, emit func(agent.Event)) (agent.Result, error) {
		return (&agent.Engine{Model: m, Client: a.client}).Answer(ctx, h, emit)
	}
	return a, nil
}
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.emit = func(e TurnEvent) { wails.EventsEmit(ctx, "course:turn", e) }
}
func (a *App) shutdown(context.Context) {
	a.mu.Lock()
	turn := a.active
	if turn != nil {
		turn.cancel()
	}
	a.mu.Unlock()
	if turn != nil {
		select {
		case <-turn.done:
		case <-time.After(5 * time.Second):
			return
		}
	}
	a.store.Close()
}
func (a *App) GetSettings() Settings {
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
	defer cancel()
	return Settings{BaseURL: c.Model.BaseURL, Model: c.Model.Name, HasAPIKey: c.Model.APIKey != "", QQStatus: a.client.Status(ctx), DataRoot: a.root}
}
func (a *App) SaveSettings(in SettingsInput) (Settings, error) {
	a.mu.Lock()
	if a.active != nil {
		a.mu.Unlock()
		return Settings{}, errors.New("请等当前回答结束后再修改设置")
	}
	next := a.cfg
	next.Model.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if strings.TrimSpace(in.APIKey) != "" {
		next.Model.APIKey = strings.TrimSpace(in.APIKey)
	}
	next.Model.Name = "deepseek-v4.1-flash"
	if next.Model.BaseURL == "https://api.deepseek.com" || next.Model.BaseURL == "https://api.deepseek.com/v1" {
		next.Model.Name = "deepseek-flash"
	}
	err := config.Save(a.root, next)
	if err == nil {
		a.cfg = next
	}
	a.mu.Unlock()
	if err != nil {
		return Settings{}, err
	}
	return a.GetSettings(), nil
}
func (a *App) InstallQQ() (Settings, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active != nil {
		return Settings{}, errors.New("请等当前回答结束后再安装连接组件")
	}
	_, err := tools.EnsureTencentCLI(a.ctx, a.root, runtime.GOOS, runtime.GOARCH, nil)
	if err != nil {
		return Settings{}, errors.New("QQ 连接组件安装失败，请检查网络或平台支持")
	}
	return Settings{BaseURL: a.cfg.Model.BaseURL, Model: a.cfg.Model.Name, HasAPIKey: a.cfg.Model.APIKey != "", QQStatus: a.client.Status(a.ctx), DataRoot: a.root}, nil
}
func (a *App) ListConversations() ([]storage.Conversation, error) {
	return a.store.Conversations(a.ctx)
}
func (a *App) GetMessages(id string) ([]storage.Message, error) { return a.store.Messages(a.ctx, id) }
func (a *App) DeleteConversation(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active != nil && a.active.conversationID == id {
		return errors.New("请先停止这条对话的回答")
	}
	return a.store.Delete(a.ctx, id)
}
func (a *App) Cancel(requestID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active != nil && a.active.id == requestID {
		a.active.cancel()
	}
}
func (a *App) Chat(conversationID, question, requestID string) (TurnResult, error) {
	question = strings.TrimSpace(question)
	if question == "" || len([]rune(question)) > 4000 {
		return TurnResult{}, errors.New("请输入 1–4000 字的问题")
	}
	if len(requestID) < 1 || len(requestID) > 100 {
		return TurnResult{}, errors.New("请求标识无效")
	}
	a.mu.Lock()
	if a.active != nil {
		a.mu.Unlock()
		return TurnResult{}, errors.New("已有一个问题正在回答，请稍等或停止")
	}
	if a.cfg.Model.APIKey == "" {
		a.mu.Unlock()
		return TurnResult{}, errors.New("请先在设置中填写模型 API Key")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	turn := &activeTurn{id: requestID, conversationID: conversationID, cancel: cancel, done: make(chan struct{})}
	a.active = turn
	model := a.cfg.Model
	a.mu.Unlock()
	defer func() { cancel(); a.mu.Lock(); a.active = nil; a.mu.Unlock(); close(turn.done) }()
	conversation, user, assistant, err := a.store.Begin(ctx, conversationID, question)
	if err != nil {
		return TurnResult{}, errors.New("未能保存问题，请重试")
	}
	a.mu.Lock()
	turn.conversationID = conversation.ID
	a.mu.Unlock()
	a.emit(TurnEvent{Kind: "start", RequestID: requestID, ConversationID: conversation.ID, Conversation: &conversation, User: &user, Assistant: &assistant})
	history, err := a.store.Messages(ctx, conversation.ID)
	var result agent.Result
	if err == nil {
		result, err = a.answer(ctx, model, history, func(e agent.Event) {
			switch e.Kind {
			case "reset":
				assistant.Content = ""
			case "delta":
				assistant.Content += e.Text
			}
			a.emit(TurnEvent{RequestID: requestID, ConversationID: conversation.ID, Kind: e.Kind, Text: e.Text})
		})
	}
	visible := TurnResult{Conversation: conversation}
	switch {
	case errors.Is(err, context.Canceled):
		assistant.State = "cancelled"
	case err != nil:
		assistant.State = "error"
		if errors.Is(err, context.DeadlineExceeded) {
			visible.Error = "这次查询用时过长，可以缩小问题范围后重试"
		} else {
			visible.Error = err.Error()
		}
	default:
		assistant.State = "complete"
		assistant.Content = result.Text
	}
	if assistant.State == "error" && assistant.Content == "" {
		assistant.Content = visible.Error
	}
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer saveCancel()
	if saveErr := a.store.Finish(saveCtx, assistant); saveErr != nil {
		return TurnResult{}, errors.New("回答未能保存到本机，请重试")
	}
	visible.Message = assistant
	a.emit(TurnEvent{Kind: "finish", RequestID: requestID, ConversationID: conversation.ID, Assistant: &assistant, Text: visible.Error})
	return visible, nil
}
