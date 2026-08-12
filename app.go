package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jaketmoon/hdu-station/internal/agent"
	"github.com/jaketmoon/hdu-station/internal/appdata"
	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/jaketmoon/hdu-station/internal/sandbox"
	stationSkills "github.com/jaketmoon/hdu-station/internal/skills"
	"github.com/jaketmoon/hdu-station/internal/storage"
	stationtools "github.com/jaketmoon/hdu-station/internal/tools"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	mu                  sync.RWMutex
	ctx                 context.Context
	dataRoot            string
	config              config.Config
	store               *storage.Store
	engine              *agent.Engine
	sandbox             sandbox.Executor
	toolRegistry        *stationtools.Registry
	campusAuthState     string
	initializationError string
	initialize          func() (*App, error)
}

func NewApp(dataRoot string, cfg config.Config) *App {
	return &App{dataRoot: dataRoot, config: cfg}
}

func newAppWithStore(dataRoot string, cfg config.Config, store *storage.Store, engine *agent.Engine, executor sandbox.Executor, registry *stationtools.Registry) *App {
	return &App{dataRoot: dataRoot, config: cfg, store: store, engine: engine, sandbox: executor, toolRegistry: registry}
}

func newRuntimeApp(initialize func() (*App, error)) *App {
	return &App{config: config.Default(), initialize: initialize}
}

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	initialize := a.initialize
	a.mu.Unlock()

	if initialize == nil {
		return
	}

	initialized, err := initialize()
	if err != nil {
		log.Printf("initialize HDU Station: %v", err)
		a.mu.Lock()
		a.initializationError = err.Error()
		a.mu.Unlock()
		return
	}

	a.mu.Lock()
	a.dataRoot = initialized.dataRoot
	a.config = initialized.config
	a.store = initialized.store
	a.engine = initialized.engine
	a.sandbox = initialized.sandbox
	a.toolRegistry = initialized.toolRegistry
	a.campusAuthState = initialized.campusAuthState
	a.initialize = nil
	a.mu.Unlock()
}

type BootstrapState struct {
	Name          string              `json:"name"`
	Version       string              `json:"version"`
	Platform      string              `json:"platform"`
	DataRoot      string              `json:"dataRoot"`
	StartupError  string              `json:"startupError,omitempty"`
	Configuration ConfigurationStatus `json:"configuration"`
}

type ConfigurationStatus struct {
	CampusConfigured    bool                    `json:"campusConfigured"`
	CampusAuth          config.CampusAuthStatus `json:"campusAuth"`
	DefaultProvider     string                  `json:"defaultProvider"`
	ConfiguredProviders []string                `json:"configuredProviders"`
}

type SettingsState struct {
	CampusConfigured bool                    `json:"campusConfigured"`
	CampusAuth       config.CampusAuthStatus `json:"campusAuth"`
	Tencent          TencentChannelStatus    `json:"tencent"`
	WebSearch        string                  `json:"webSearch"`
	Providers        []ProviderSettings      `json:"providers"`
}

// TencentChannelStatus reports only local prerequisites owned by Station. The
// official CLI remains the owner of login state; this DTO never exposes its
// token, state file, or raw command output.
type TencentChannelStatus struct {
	CLIInstalled           bool   `json:"cliInstalled"`
	ChannelIndexConfigured bool   `json:"channelIndexConfigured"`
	SearchAvailable        bool   `json:"searchAvailable"`
	Notice                 string `json:"notice"`
}

type ProviderSettings struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Protocol   string `json:"protocol"`
	BaseURL    string `json:"baseURL"`
	Model      string `json:"model"`
	Configured bool   `json:"configured"`
}

func (a *App) Bootstrap() BootstrapState {
	a.mu.RLock()
	defer a.mu.RUnlock()

	status := a.config.Status()
	return BootstrapState{
		Name:         "HDU Station",
		Version:      "0.1.0-dev",
		Platform:     runtime.GOOS + "/" + runtime.GOARCH,
		DataRoot:     a.dataRoot,
		StartupError: a.initializationError,
		Configuration: ConfigurationStatus{
			CampusConfigured:    status.CampusConfigured,
			CampusAuth:          a.campusAuthStatusLocked(),
			DefaultProvider:     status.DefaultProvider,
			ConfiguredProviders: status.ConfiguredProviders,
		},
	}
}

func (a *App) Settings() SettingsState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	status := a.config.Status()
	providers := make([]ProviderSettings, 0, len(a.config.Models.Providers))
	for name, provider := range a.config.Models.Providers {
		providers = append(providers, ProviderSettings{
			Name:       name,
			Type:       provider.Type,
			Protocol:   provider.Protocol,
			BaseURL:    provider.BaseURL,
			Model:      provider.Model,
			Configured: provider.APIKey != "" && provider.Model != "",
		})
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].Name < providers[j].Name })
	return SettingsState{
		CampusConfigured: status.CampusConfigured,
		CampusAuth:       a.campusAuthStatusLocked(),
		Tencent:          a.tencentChannelStatusLocked(),
		WebSearch:        a.config.WebSearch.Provider,
		Providers:        providers,
	}
}

func (a *App) TencentChannelStatus() TencentChannelStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.tencentChannelStatusLocked()
}

func (a *App) tencentChannelStatusLocked() TencentChannelStatus {
	if strings.TrimSpace(a.dataRoot) == "" {
		return TencentChannelStatus{Notice: "Station 尚未完成本地初始化。"}
	}
	skill, err := stationSkills.LoadCourseSelection(a.dataRoot)
	if err != nil {
		return TencentChannelStatus{Notice: "课程 Skill 无法读取，频道检索已关闭。"}
	}
	installed := stationtools.TencentCLIPathAt(a.dataRoot) != ""
	configured := len(skill.TencentGuildIDs) > 0
	status := TencentChannelStatus{
		CLIInstalled:           installed,
		ChannelIndexConfigured: configured,
		SearchAvailable:        installed && configured,
	}
	switch {
	case !installed:
		status.Notice = "腾讯频道 CLI 尚未安装；登录凭据由官方 CLI 管理。"
	case !configured:
		status.Notice = "腾讯频道 CLI 已安装，但课程 Skill 尚未配置可验证的数字 guild ID。"
	default:
		status.Notice = "频道只读检索已具备本地前置条件；登录状态将在首次检索时由官方 CLI 验证。"
	}
	return status
}

func (a *App) SaveModelProvider(name, apiKey, model, baseURL, protocol string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	provider, ok := a.config.Models.Providers[name]
	if !ok {
		return fmt.Errorf("model provider %q is not configured", name)
	}
	if strings.TrimSpace(apiKey) != "" {
		provider.APIKey = strings.TrimSpace(apiKey)
	}
	if strings.TrimSpace(model) != "" {
		provider.Model = strings.TrimSpace(model)
	}
	if strings.TrimSpace(baseURL) != "" {
		provider.BaseURL = strings.TrimSpace(baseURL)
	}
	if strings.TrimSpace(protocol) != "" {
		provider.Protocol = strings.TrimSpace(protocol)
	}
	candidate := a.config
	candidate.Models.Providers = cloneProviders(a.config.Models.Providers)
	candidate.Models.Providers[name] = provider
	if provider.APIKey != "" && provider.Model != "" {
		candidate.Models.Default = name
	}
	if err := candidate.Validate(); err != nil {
		return fmt.Errorf("validate model settings: %w", err)
	}
	if err := config.NewStore(a.dataRoot).Save(candidate); err != nil {
		return fmt.Errorf("save model settings: %w", err)
	}
	prompt := ""
	if a.engine != nil {
		prompt = a.engine.SystemPrompt()
	}
	a.config = candidate
	a.engine = agent.New(candidate, http.DefaultClient)
	a.engine.SetSystemPrompt(prompt)
	return nil
}

func (a *App) SaveCampusKey(key string) error {
	return a.saveCampusKey(key)
}

func (a *App) saveCampusKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("campus key is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	candidate := a.config
	candidate.Campus.Key = key
	if err := candidate.Validate(); err != nil {
		return fmt.Errorf("validate campus settings: %w", err)
	}
	if err := config.NewStore(a.dataRoot).Save(candidate); err != nil {
		return fmt.Errorf("save campus settings: %w", err)
	}
	skill, skillErr := stationSkills.LoadCourseSelection(a.dataRoot)
	if skillErr != nil {
		return fmt.Errorf("reload course-selection skill: %w", skillErr)
	}
	registry, err := newToolRegistry(a.dataRoot, candidate, http.DefaultClient, a.sandbox, skill.TencentGuildIDs)
	if err != nil {
		return fmt.Errorf("reload campus tools: %w", err)
	}
	a.config = candidate
	a.toolRegistry = registry
	a.campusAuthState = ""
	return nil
}

// ImportHduhelpCLICredential imports the current user's hduhelp-cli PAT only
// when the user explicitly requests it from Station settings. It does not
// establish a background synchronization with another application's config.
func (a *App) ImportHduhelpCLICredential() error {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolve hduhelp-cli configuration: %w", err)
	}
	return a.importHduhelpCLICredentialAt(filepath.Join(configRoot, "hduhelp", "config.json"))
}

func (a *App) importHduhelpCLICredentialAt(path string) error {
	key, err := config.LoadHduhelpCLIPAT(path, time.Now())
	if err != nil {
		return err
	}
	return a.saveCampusKey(key)
}

// ClearCampusCredential removes only the Station-owned Neo credential. It
// deliberately leaves conversations, model keys, Sandbox state and Tencent's
// connector-owned login untouched.
func (a *App) ClearCampusCredential() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if strings.TrimSpace(a.dataRoot) == "" {
		return errors.New("HDU Station data root is not initialized")
	}
	candidate := a.config
	candidate.Campus.Key = ""
	if err := candidate.Validate(); err != nil {
		return fmt.Errorf("validate campus settings: %w", err)
	}
	skill, err := stationSkills.LoadCourseSelection(a.dataRoot)
	if err != nil {
		return fmt.Errorf("reload course-selection skill: %w", err)
	}
	registry, err := newToolRegistry(a.dataRoot, candidate, http.DefaultClient, a.sandbox, skill.TencentGuildIDs)
	if err != nil {
		return fmt.Errorf("reload campus tools: %w", err)
	}
	if err := config.NewStore(a.dataRoot).Save(candidate); err != nil {
		return fmt.Errorf("clear campus settings: %w", err)
	}
	a.config = candidate
	a.toolRegistry = registry
	a.campusAuthState = ""
	return nil
}

func (a *App) shutdown(_ context.Context) {
	a.mu.Lock()
	store := a.store
	executor := a.sandbox
	a.store = nil
	a.engine = nil
	a.sandbox = nil
	a.toolRegistry = nil
	a.mu.Unlock()
	if lifecycle, ok := executor.(sandbox.Lifecycle); ok {
		stopContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := lifecycle.Stop(stopContext); err != nil {
			log.Printf("stop HDU Station Sandbox: %v", err)
		}
		cancel()
	}
	if store != nil {
		if err := store.Close(); err != nil {
			log.Printf("close HDU Station storage: %v", err)
		}
	}
}

func (a *App) Conversations() ([]storage.Conversation, error) {
	store, ctx, err := a.runtimeStore()
	if err != nil {
		return nil, err
	}
	return store.ListConversations(ctx)
}

func (a *App) CreateConversation(title string) (storage.Conversation, error) {
	store, ctx, err := a.runtimeStore()
	if err != nil {
		return storage.Conversation{}, err
	}
	return store.CreateConversation(ctx, title)
}

func (a *App) Messages(conversationID string) ([]storage.Message, error) {
	store, ctx, err := a.runtimeStore()
	if err != nil {
		return nil, err
	}
	return store.ListMessages(ctx, conversationID)
}

func (a *App) AppendMessage(conversationID, role, content string) (storage.Message, error) {
	store, ctx, err := a.runtimeStore()
	if err != nil {
		return storage.Message{}, err
	}
	return store.AppendMessage(ctx, conversationID, role, content)
}

func (a *App) Chat(conversationID, prompt string) (storage.Message, error) {
	return a.chat(conversationID, prompt, nil)
}

// ChatStream is the desktop binding used by the UI. It returns the persisted
// assistant message just like Chat, while emitting bounded text deltas over
// the Wails event bridge during provider streaming.
func (a *App) ChatStream(conversationID, prompt string) (storage.Message, error) {
	return a.chat(conversationID, prompt, func(delta string) {
		a.emitChatToken(conversationID, delta)
	})
}

func (a *App) chat(conversationID, prompt string, textObserver agent.TextObserver) (storage.Message, error) {
	store, ctx, err := a.runtimeStore()
	if err != nil {
		return storage.Message{}, err
	}
	engine, err := a.runtimeEngine()
	if err != nil {
		return storage.Message{}, err
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return storage.Message{}, errors.New("message content is required")
	}
	if _, err := store.AppendMessage(ctx, conversationID, string(agent.RoleUser), prompt); err != nil {
		return storage.Message{}, fmt.Errorf("save user message: %w", err)
	}
	persisted, err := store.ListMessages(ctx, conversationID)
	if err != nil {
		return storage.Message{}, fmt.Errorf("load conversation for model: %w", err)
	}
	messages := make([]agent.Message, 0, len(persisted))
	for _, message := range persisted {
		messages = append(messages, agent.Message{Role: agent.Role(message.Role), Content: message.Content})
	}
	toolObserver := func(event agent.ToolEvent) {
		a.observeCampusAuthEvent(event)
		if _, auditErr := store.AppendToolAudit(ctx, conversationID, event.ToolName, event.Status, event.Detail); auditErr != nil {
			log.Printf("record HDU Station tool audit: %v", auditErr)
		}
	}
	var response string
	if textObserver == nil {
		response, err = engine.RespondWithObserver(ctx, messages, a.runtimeToolRegistry(), toolObserver)
	} else {
		response, err = engine.RespondWithStream(ctx, messages, a.runtimeToolRegistry(), toolObserver, textObserver)
	}
	if err != nil {
		return storage.Message{}, fmt.Errorf("complete model response: %w", err)
	}
	assistant, err := store.AppendMessage(ctx, conversationID, string(agent.RoleAssistant), response)
	if err != nil {
		return storage.Message{}, fmt.Errorf("save assistant message: %w", err)
	}
	return assistant, nil
}

func (a *App) emitChatToken(conversationID, delta string) {
	if delta == "" {
		return
	}
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()
	if ctx == nil {
		return
	}
	wailsRuntime.EventsEmit(ctx, "station:chat-token", map[string]string{
		"conversationId": conversationID,
		"text":           delta,
	})
}

func (a *App) SandboxStatus() (sandbox.Status, error) {
	a.mu.RLock()
	executor := a.sandbox
	a.mu.RUnlock()
	if executor == nil {
		return sandbox.Status{}, errors.New("HDU Station Sandbox is not initialized")
	}
	return executor.Status(context.Background()), nil
}

func (a *App) ToolAudits(conversationID string) ([]storage.ToolAudit, error) {
	store, ctx, err := a.runtimeStore()
	if err != nil {
		return nil, err
	}
	return store.ListToolAudits(ctx, conversationID)
}

func (a *App) InstallSandbox() error {
	lifecycle, ctx, err := a.runtimeSandboxLifecycle()
	if err != nil {
		return err
	}
	return lifecycle.Install(ctx)
}

func (a *App) InstallTencentCLI() error {
	a.mu.RLock()
	root, ctx := a.dataRoot, a.ctx
	cfg, executor := a.config, a.sandbox
	a.mu.RUnlock()
	if strings.TrimSpace(root) == "" {
		return errors.New("HDU Station data root is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	binary, err := stationtools.EnsureTencentCLI(ctx, root, runtime.GOOS, runtime.GOARCH, http.DefaultClient)
	if err != nil {
		return err
	}
	skill, err := stationSkills.LoadCourseSelection(root)
	if err != nil {
		return fmt.Errorf("load course-selection skill: %w", err)
	}
	registry, err := newToolRegistryWithTencentBinary(root, cfg, http.DefaultClient, executor, binary, skill.TencentGuildIDs)
	if err != nil {
		return fmt.Errorf("reload Tencent Channel tools: %w", err)
	}
	a.mu.Lock()
	a.toolRegistry = registry
	a.mu.Unlock()
	return nil
}

func (a *App) StartSandbox() error {
	lifecycle, ctx, err := a.runtimeSandboxLifecycle()
	if err != nil {
		return err
	}
	return lifecycle.Start(ctx)
}

func (a *App) StopSandbox() error {
	lifecycle, ctx, err := a.runtimeSandboxLifecycle()
	if err != nil {
		return err
	}
	return lifecycle.Stop(ctx)
}

func (a *App) PurgeSandbox() error {
	lifecycle, ctx, err := a.runtimeSandboxLifecycle()
	if err != nil {
		return err
	}
	return lifecycle.Purge(ctx)
}

// ClearAllData stops and removes Station-owned state. Connector-owned
// credentials, including Tencent Channel CLI credentials, remain untouched.
// The desktop process must be restarted after this call because its in-memory
// store and configuration have been closed and removed.
func (a *App) ClearAllData() error {
	a.mu.RLock()
	root := a.dataRoot
	store := a.store
	executor := a.sandbox
	a.mu.RUnlock()
	if strings.TrimSpace(root) == "" {
		return errors.New("HDU Station data root is not initialized")
	}
	if lifecycle, ok := executor.(sandbox.Lifecycle); ok {
		clearContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if err := lifecycle.Purge(clearContext); err != nil && !errors.Is(err, sandbox.ErrUnavailable) {
			cancel()
			return fmt.Errorf("purge HDU Station Sandbox before clearing data: %w", err)
		}
		cancel()
	}
	if store != nil {
		if err := store.Close(); err != nil {
			return fmt.Errorf("close HDU Station storage before clearing data: %w", err)
		}
	}
	if err := appdata.ClearRoot(root); err != nil {
		return err
	}
	a.mu.Lock()
	a.store = nil
	a.engine = nil
	a.sandbox = nil
	a.toolRegistry = nil
	a.initializationError = "local data cleared; restart HDU Station to continue"
	a.mu.Unlock()
	return nil
}

func (a *App) ReadOnlyTools() []stationtools.Definition {
	a.mu.RLock()
	registry := a.toolRegistry
	a.mu.RUnlock()
	if registry == nil {
		return []stationtools.Definition{}
	}
	return registry.ReadOnlyDefinitions()
}

func (a *App) Tools() []stationtools.Definition {
	a.mu.RLock()
	registry := a.toolRegistry
	a.mu.RUnlock()
	if registry == nil {
		return []stationtools.Definition{}
	}
	return registry.Definitions()
}

func (a *App) CallReadOnlyTool(name, arguments string) (stationtools.Result, error) {
	a.mu.RLock()
	registry, ctx := a.toolRegistry, a.ctx
	a.mu.RUnlock()
	if registry == nil {
		return stationtools.Result{}, errors.New("HDU Station tools are not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := registry.CallReadOnly(ctx, name, json.RawMessage(arguments))
	if isNeoAcademicTool(name) {
		if err == nil {
			a.observeCampusAuthEvent(agent.ToolEvent{ToolName: name, Status: "completed"})
		} else {
			a.observeCampusAuthError(err)
		}
	}
	return result, err
}

func (a *App) campusAuthStatusLocked() config.CampusAuthStatus {
	status := a.config.CampusAuthStatus()
	if !status.Configured || a.campusAuthState == "" {
		return status
	}
	status.State = a.campusAuthState
	switch a.campusAuthState {
	case config.CampusAuthPATVerified:
		status.Notice = "最近一次校园只读请求已成功；校园 Key 仅保留在宿主侧，不会交给 Sandbox。"
	case config.CampusAuthPATRejected:
		status.Notice = "最近一次校园请求拒绝了当前 Key；请重新配置校园 Key。"
	case config.CampusAuthScopeMissing:
		status.Notice = "最近一次校园请求缺少所需权限；请在 HDUHelp 令牌页面补充对应的校园只读 scope。"
	case config.CampusAuthUnavailable:
		status.Notice = "校园服务暂时不可用；不能据此判断校园 Key 已失效。"
	}
	return status
}

func (a *App) observeCampusAuthEvent(event agent.ToolEvent) {
	if !isNeoAcademicTool(event.ToolName) {
		return
	}
	if event.Status == "completed" {
		a.mu.Lock()
		if a.config.Campus.Key != "" {
			a.campusAuthState = config.CampusAuthPATVerified
		}
		a.mu.Unlock()
		return
	}
	if event.Status == "failed" {
		a.observeCampusAuthError(errors.New(event.Detail))
	}
}

func (a *App) observeCampusAuthError(err error) {
	if err == nil {
		return
	}
	message := strings.ToLower(err.Error())
	state := ""
	if strings.Contains(message, "missing the required scope") || strings.Contains(message, "scope_missing") || strings.Contains(message, "insufficient_scope") || strings.Contains(message, "scope_insufficient") {
		state = config.CampusAuthScopeMissing
	} else if message == "credential rejected" || strings.Contains(message, "http 401") || strings.Contains(message, "http 403") || strings.Contains(message, "unauthorized") || strings.Contains(message, "forbidden") || strings.Contains(message, "invalid token") || strings.Contains(message, "invalid ai token") {
		state = config.CampusAuthPATRejected
	} else if message == "upstream unavailable" || strings.Contains(message, "hduhelp neo") || strings.Contains(message, "timeout") || strings.Contains(message, "connection") {
		state = config.CampusAuthUnavailable
	}
	if state == "" {
		return
	}
	a.mu.Lock()
	if a.config.Campus.Key != "" {
		a.campusAuthState = state
	}
	a.mu.Unlock()
}

func isNeoAcademicTool(name string) bool {
	switch name {
	case "hdu_academic_class_search", "hdu_academic_course_selection", "hdu_academic_schedule", "hdu_academic_schedule_now":
		return true
	default:
		return false
	}
}

func (a *App) runtimeStore() (*storage.Store, context.Context, error) {
	a.mu.RLock()
	store, ctx := a.store, a.ctx
	a.mu.RUnlock()
	if store == nil {
		return nil, nil, errors.New("HDU Station storage is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return store, ctx, nil
}

func (a *App) runtimeEngine() (*agent.Engine, error) {
	a.mu.RLock()
	engine := a.engine
	a.mu.RUnlock()
	if engine == nil {
		return nil, errors.New("HDU Station model engine is not initialized")
	}
	return engine, nil
}

func (a *App) runtimeToolRegistry() *stationtools.Registry {
	a.mu.RLock()
	registry := a.toolRegistry
	a.mu.RUnlock()
	return registry
}

func (a *App) runtimeSandboxLifecycle() (sandbox.Lifecycle, context.Context, error) {
	a.mu.RLock()
	executor, ctx := a.sandbox, a.ctx
	a.mu.RUnlock()
	if executor == nil {
		return nil, nil, errors.New("HDU Station Sandbox is not initialized")
	}
	lifecycle, ok := executor.(sandbox.Lifecycle)
	if !ok {
		return nil, nil, errors.New("HDU Station Sandbox does not support lifecycle management")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return lifecycle, ctx, nil
}

func cloneProviders(providers map[string]config.ProviderConfig) map[string]config.ProviderConfig {
	cloned := make(map[string]config.ProviderConfig, len(providers))
	for name, provider := range providers {
		cloned[name] = provider
	}
	return cloned
}
