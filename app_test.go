package main

import (
	"context"
	"encoding/json"
	"github.com/jaketmoon/hdu-station/internal/agent"
	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/jaketmoon/hdu-station/internal/storage"
	"strings"
	"testing"
	"time"
)

func testApp(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	cfg := config.Default()
	cfg.Model.APIKey = "private-key-sentinel"
	if err := config.Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	a, err := createApplicationAt(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.shutdown(context.Background()) })
	return a
}
func TestChatCancellationIsRequestScopedAndPersistsPartialAnswer(t *testing.T) {
	a := testApp(t)
	entered := make(chan struct{})
	finished := make(chan TurnResult, 1)
	fail := make(chan error, 1)
	a.answer = func(ctx context.Context, _ config.Model, _ []storage.Message, emit func(agent.Event)) (agent.Result, error) {
		emit(agent.Event{Kind: "delta", Text: "已经查到一些资料"})
		close(entered)
		<-ctx.Done()
		return agent.Result{}, ctx.Err()
	}
	go func() {
		r, err := a.Chat("", "通识选修有什么水课？", "one")
		if err != nil {
			fail <- err
		}
		finished <- r
	}()
	<-entered
	if _, err := a.Chat("", "另一个问题", "two"); err == nil {
		t.Fatal("overlapping turn accepted")
	}
	a.Cancel("old-request")
	select {
	case <-finished:
		t.Fatal("stale cancel stopped active turn")
	case <-time.After(10 * time.Millisecond):
	}
	a.Cancel("one")
	select {
	case err := <-fail:
		t.Fatal(err)
	case r := <-finished:
		if r.Message.State != "cancelled" || r.Message.Content != "已经查到一些资料" {
			t.Fatal("partial answer lost")
		}
		messages, _ := a.GetMessages(r.Conversation.ID)
		if len(messages) != 2 || messages[1].State != "cancelled" {
			t.Fatal("cancelled state not persisted")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel failed")
	}
}
func TestSettingsNeverReturnKeyAndBlankKeyKeepsExisting(t *testing.T) {
	a := testApp(t)
	settings, err := a.SaveSettings(SettingsInput{BaseURL: "https://api.deepseek.com"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(settings)
	if strings.Contains(string(data), "private-key-sentinel") {
		t.Fatal("public settings exposed key")
	}
	saved, err := config.Load(a.root)
	if err != nil || saved.Model.APIKey != "private-key-sentinel" {
		t.Fatal("blank key destroyed existing credential")
	}
}

func TestSourceSettingsPreserveReplaceAndClearPrivateTokens(t *testing.T) {
	a := testApp(t)
	in := SettingsInput{BaseURL: "https://api.deepseek.com",
		Zanao:       &ZanaoInput{SchoolAlias: "test", Token: "zanao-private-sentinel"},
		Xiaohongshu: &XiaohongshuInput{BaseURL: "http://127.0.0.1:18060", AuthToken: "service-private-sentinel"},
	}
	settings, err := a.SaveSettings(in)
	if err != nil || !settings.Zanao.HasToken || !settings.Xiaohongshu.HasAuthToken {
		t.Fatal("source credentials not saved")
	}
	data, _ := json.Marshal(settings)
	if strings.Contains(string(data), "private-sentinel") {
		t.Fatal("source credentials returned to interface")
	}
	in.Zanao.Token, in.Xiaohongshu.AuthToken = "", ""
	_, err = a.SaveSettings(in)
	cfg, _ := config.Load(a.root)
	if err != nil || cfg.Sources.Zanao.Token != "zanao-private-sentinel" || cfg.Sources.Xiaohongshu.AuthToken != "service-private-sentinel" {
		t.Fatal("blank fields did not preserve source credentials")
	}
	_, err = a.SaveSettings(SettingsInput{BaseURL: in.BaseURL})
	cfg, _ = config.Load(a.root)
	if err != nil || cfg.Sources.Zanao.Token == "" {
		t.Fatal("legacy caller removed source configuration")
	}
	in.Xiaohongshu.BaseURL = "https://remote.example:18060"
	if _, err := a.SaveSettings(in); err == nil {
		t.Fatal("remote service accepted")
	}
	cfg, _ = config.Load(a.root)
	if cfg.Sources.Xiaohongshu.BaseURL != "http://127.0.0.1:18060" {
		t.Fatal("failed save changed configuration")
	}
	in.Xiaohongshu.BaseURL = "http://127.0.0.1:18060"
	in.Zanao.ClearToken, in.Xiaohongshu.ClearAuthToken = true, true
	settings, err = a.SaveSettings(in)
	if err != nil || settings.Zanao.HasToken || settings.Xiaohongshu.HasAuthToken {
		t.Fatal("explicit clearing failed")
	}
	cfg, _ = config.Load(a.root)
	if cfg.Sources.Zanao.Token != "" || cfg.Sources.Xiaohongshu.AuthToken != "" {
		t.Fatal("cleared source credential persisted")
	}
}
