package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/agent"
	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/jaketmoon/hdu-station/internal/sandbox"
	"github.com/jaketmoon/hdu-station/internal/storage"
	stationtools "github.com/jaketmoon/hdu-station/internal/tools"
)

func TestBootstrapIdentifiesLocalDesktopBuild(t *testing.T) {
	cfg := config.Default()
	cfg.Campus.Key = "campus-test-key"
	provider := cfg.Models.Providers["openai"]
	provider.APIKey = "model-test-key"
	provider.Model = "gpt-test"
	cfg.Models.Providers["openai"] = provider
	cfg.Models.Default = "openai"
	state := NewApp("/tmp/station-test", cfg).Bootstrap()

	if state.Name != "HDU Station" {
		t.Fatalf("unexpected app name %q", state.Name)
	}
	if state.Version == "" {
		t.Fatal("version must not be empty")
	}
	wantPlatform := runtime.GOOS + "/" + runtime.GOARCH
	if state.Platform != wantPlatform {
		t.Fatalf("platform = %q, want %q", state.Platform, wantPlatform)
	}
	if state.DataRoot != "/tmp/station-test" {
		t.Fatalf("data root = %q", state.DataRoot)
	}
	if !state.Configuration.CampusConfigured || state.Configuration.DefaultProvider != "openai" {
		t.Fatalf("unexpected configuration status: %#v", state.Configuration)
	}
	if state.Configuration.CampusAuth.Method != "pat" || state.Configuration.CampusAuth.DeviceAuthorization != "unavailable" {
		t.Fatalf("unexpected campus auth status: %#v", state.Configuration.CampusAuth)
	}
}

func TestRuntimeAppDefersInitializationUntilStartup(t *testing.T) {
	called := false
	app := newRuntimeApp(func() (*App, error) {
		called = true
		return NewApp("/tmp/runtime-station", config.Default()), nil
	})

	if called {
		t.Fatal("runtime app initialized before Wails startup")
	}
	app.startup(context.Background())
	if !called {
		t.Fatal("runtime app did not initialize during Wails startup")
	}
	if got := app.Bootstrap().DataRoot; got != "/tmp/runtime-station" {
		t.Fatalf("data root = %q", got)
	}
}

func TestRuntimeAppReportsInitializationFailure(t *testing.T) {
	app := newRuntimeApp(func() (*App, error) {
		return nil, errors.New("configuration unavailable")
	})

	app.startup(context.Background())
	if got := app.Bootstrap().StartupError; got != "configuration unavailable" {
		t.Fatalf("startup error = %q", got)
	}
}

func TestAppExposesPersistentSessionBindings(t *testing.T) {
	sessions, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer sessions.Close()

	conversation, err := sessions.CreateConversation(context.Background(), "选课助手")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := stationtools.NewReadOnlyRegistry(stationtools.NewWebFetchTool(nil))
	if err != nil {
		t.Fatal(err)
	}
	app := newAppWithStore("/tmp/station-test", config.Default(), sessions, nil, sandbox.New("linux", "amd64"), registry)
	if _, err := app.AppendMessage(conversation.ID, "user", "帮我看看课程"); err != nil {
		t.Fatal(err)
	}
	messages, err := app.Messages(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Content != "帮我看看课程" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
}

func TestAppChatStoresModelResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"模型回复"}}]}`))
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Models.Default = "openai"
	provider := cfg.Models.Providers["openai"]
	provider.APIKey = "test-key"
	provider.Model = "test-model"
	provider.BaseURL = server.URL + "/v1"
	provider.Protocol = "chat_completions"
	cfg.Models.Providers["openai"] = provider

	sessions, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer sessions.Close()
	conversation, err := sessions.CreateConversation(context.Background(), "选课助手")
	if err != nil {
		t.Fatal(err)
	}
	app := newAppWithStore("/tmp/station-test", cfg, sessions, agent.New(cfg, server.Client()), sandbox.New("linux", "amd64"), nil)

	assistant, err := app.Chat(conversation.ID, "帮我看看课程")
	if err != nil {
		t.Fatal(err)
	}
	if assistant.Role != "assistant" || assistant.Content != "模型回复" {
		t.Fatalf("unexpected assistant message: %#v", assistant)
	}
	messages, err := app.Messages(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("unexpected conversation: %#v", messages)
	}
}

func TestAppSettingsSaveProviderKeepsSecretOutOfSettingsView(t *testing.T) {
	root := t.TempDir()
	seed := config.Default()
	seed.Campus.Key = "campus-test-key"
	app, err := createApplicationAt(root, seed)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(context.Background())

	if err := app.SaveModelProvider("openai", "secret-test-key", "test-model", "https://example.test/v1", "responses"); err != nil {
		t.Fatal(err)
	}
	settings := app.Settings()
	var configured bool
	for _, provider := range settings.Providers {
		if provider.Name == "openai" {
			configured = provider.Configured
		}
	}
	if !configured {
		t.Fatal("saved provider should be reported as configured")
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret-test-key") {
		t.Fatal("settings view must not contain the API key")
	}
	if strings.Contains(string(encoded), "campus-test-key") {
		t.Fatal("settings view must not contain the campus key")
	}
	bootstrap, err := json.Marshal(app.Bootstrap())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bootstrap), "campus-test-key") {
		t.Fatal("bootstrap view must not contain the campus key")
	}
	reloaded, err := config.NewStore(root).Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Models.Providers["openai"].APIKey != "secret-test-key" {
		t.Fatal("API key was not persisted in local YAML")
	}
}

func TestClearCampusCredentialKeepsOtherLocalData(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	cfg.Campus.Key = "campus-secret"
	app, err := createApplicationAt(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(context.Background())

	if err := app.ClearCampusCredential(); err != nil {
		t.Fatal(err)
	}
	if app.Settings().CampusAuth.State != "not_configured" {
		t.Fatalf("campus auth state = %#v", app.Settings().CampusAuth)
	}
	reloaded, err := config.NewStore(root).Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Campus.Key != "" {
		t.Fatal("campus key was not removed")
	}
	if _, err := os.Stat(filepath.Join(root, "station.db")); err != nil {
		t.Fatalf("clearing campus key removed unrelated Station data: %v", err)
	}
}

func TestImportHduhelpCLICredentialCopiesOnlyValidatedPATIntoStationConfig(t *testing.T) {
	root := t.TempDir()
	app, err := createApplicationAt(root, config.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(context.Background())
	credentialPath := filepath.Join(t.TempDir(), "hduhelp-config.json")
	const token = "hduhelp_pat_0123456789012345678901234567890123456789"
	if err := os.WriteFile(credentialPath, []byte(`{"server":"https://api.hduhelp.com","token":"`+token+`","expires_at":"2030-01-01T00:00:00Z","scopes":["academic:course:read"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := app.importHduhelpCLICredentialAt(credentialPath); err != nil {
		t.Fatal(err)
	}
	if app.Settings().CampusAuth.State != config.CampusAuthPATConfigured {
		t.Fatalf("campus auth = %#v", app.Settings().CampusAuth)
	}
	reloaded, err := config.NewStore(root).Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Campus.Key != token {
		t.Fatal("Station did not save imported PAT")
	}
	for _, view := range []any{app.Settings(), app.Bootstrap()} {
		encoded, marshalErr := json.Marshal(view)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if strings.Contains(string(encoded), token) {
			t.Fatal("imported PAT escaped a Station status DTO")
		}
	}
}

func TestCampusAuthStatusTracksVerificationWithoutEchoingPAT(t *testing.T) {
	cfg := config.Default()
	cfg.Campus.Key = "campus-secret"
	app := NewApp(t.TempDir(), cfg)
	if got := app.Settings().CampusAuth.State; got != config.CampusAuthPATConfigured {
		t.Fatalf("initial campus auth state = %q", got)
	}
	app.observeCampusAuthEvent(agent.ToolEvent{ToolName: "hdu_academic_schedule", Status: "completed"})
	status := app.Settings().CampusAuth
	if status.State != config.CampusAuthPATVerified || strings.Contains(status.Notice, "campus-secret") {
		t.Fatalf("unexpected verified campus auth status: %#v", status)
	}
	app.observeCampusAuthError(errors.New("credential rejected"))
	if got := app.Bootstrap().Configuration.CampusAuth.State; got != config.CampusAuthPATRejected {
		t.Fatalf("rejected campus auth state = %q", got)
	}
	app.observeCampusAuthError(errors.New("HDUHelp Neo academic tool failed: invalid ai token"))
	if got := app.Settings().CampusAuth.State; got != config.CampusAuthPATRejected {
		t.Fatalf("tool-level rejected campus auth state = %q", got)
	}
	app.observeCampusAuthError(errors.New("HDUHelp Neo authorization is missing the required scope"))
	if got := app.Settings().CampusAuth.State; got != config.CampusAuthScopeMissing {
		t.Fatalf("scope-missing campus auth state = %q", got)
	}
	app.observeCampusAuthError(errors.New("upstream unavailable"))
	if got := app.Settings().CampusAuth.State; got != config.CampusAuthUnavailable {
		t.Fatalf("unavailable campus auth state = %q", got)
	}
}

func TestTencentChannelStatusDoesNotClaimReadinessWithoutLocalPrerequisites(t *testing.T) {
	root := t.TempDir()
	app, err := createApplicationAt(root, config.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(context.Background())
	status := app.TencentChannelStatus()
	if status.CLIInstalled || !status.ChannelIndexConfigured || status.SearchAvailable {
		t.Fatalf("uninstalled but configured Tencent channel should not be ready: %#v", status)
	}
	if !strings.Contains(status.Notice, "尚未安装") {
		t.Fatalf("unexpected Tencent status notice: %#v", status)
	}
}

func TestClearAllDataRemovesStationRootAndLeavesProcessWithoutStore(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "HDU Station")
	app, err := createApplicationAt(root, config.Default())
	if err != nil {
		t.Fatal(err)
	}
	if err := app.ClearAllData(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("Station data root still exists: %v", err)
	}
	if _, err := app.Conversations(); err == nil || !strings.Contains(err.Error(), "storage is not initialized") {
		t.Fatalf("cleared app should not use removed storage: %v", err)
	}
}

type appTestLifecycleExecutor struct {
	status   sandbox.Status
	purgeErr error
	stopErr  error
	purged   bool
	stopped  bool
}

func (executor *appTestLifecycleExecutor) Status(context.Context) sandbox.Status {
	return executor.status
}

func (*appTestLifecycleExecutor) Execute(context.Context, sandbox.Request) (sandbox.Result, error) {
	return sandbox.Result{}, errors.New("test executor does not execute")
}

func (*appTestLifecycleExecutor) Install(context.Context) error { return nil }
func (*appTestLifecycleExecutor) Start(context.Context) error   { return nil }

func (executor *appTestLifecycleExecutor) Stop(context.Context) error {
	executor.stopped = true
	return executor.stopErr
}

func (executor *appTestLifecycleExecutor) Purge(context.Context) error {
	executor.purged = true
	if executor.purgeErr == nil {
		// Lifecycle.Purge owns the stop-before-delete sequence inside the
		// backend; App should not call Stop a second time.
		executor.stopped = true
	}
	return executor.purgeErr
}

func TestClearAllDataDoesNotDeleteRootWhenSandboxPurgeFails(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "HDU Station")
	store, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	executor := &appTestLifecycleExecutor{
		status:   sandbox.Status{Supported: true, Ready: true},
		purgeErr: errors.New("sandbox still running"),
	}
	app := newAppWithStore(root, config.Default(), store, nil, executor, nil)

	if err := app.ClearAllData(); err == nil || !strings.Contains(err.Error(), "purge HDU Station Sandbox") {
		t.Fatalf("ClearAllData error = %v", err)
	}
	if !executor.purged {
		t.Fatal("ClearAllData did not purge the Sandbox before deleting data")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("Station root was deleted after Sandbox purge failed: %v", err)
	}
	if _, err := app.Conversations(); err != nil {
		t.Fatalf("failed cleanup should keep storage usable: %v", err)
	}

	executor.purgeErr = nil
	if err := app.ClearAllData(); err != nil {
		t.Fatal(err)
	}
	if !executor.purged || !executor.stopped {
		t.Fatalf("successful cleanup did not complete Sandbox lifecycle: %#v", executor)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("Station root still exists after successful cleanup: %v", err)
	}
}
