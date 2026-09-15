package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	a.mu.Lock()
	conversationID := a.active["one"].conversationID
	a.mu.Unlock()
	if _, err := a.Chat(conversationID, "另一个问题", "two"); err == nil {
		t.Fatal("overlapping turn in the same conversation accepted")
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

func TestTenConversationsRunIndependentlyAndReleaseOnlyTheirSlot(t *testing.T) {
	a := testApp(t)
	entered := make(chan string, 11)
	type outcome struct {
		id     string
		result TurnResult
		err    error
	}
	finished := make(chan outcome, 11)
	release := make(chan struct{})
	a.answer = func(ctx context.Context, _ config.Model, history []storage.Message, emit func(agent.Event)) (agent.Result, error) {
		question := history[len(history)-2].Content
		emit(agent.Event{Kind: "delta", Text: "回答：" + question})
		entered <- question
		select {
		case <-ctx.Done():
			return agent.Result{}, ctx.Err()
		case <-release:
			return agent.Result{Text: "完成：" + question}, nil
		}
	}
	start := func(id string) { go func() { r, err := a.Chat("", id, id); finished <- outcome{id, r, err} }() }
	for i := 0; i < maxConcurrentConversations; i++ {
		start(fmt.Sprintf("request-%d", i))
	}
	for i := 0; i < maxConcurrentConversations; i++ {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("ten conversations did not start concurrently")
		}
	}
	if _, err := a.Chat("", "overflow", "overflow"); err == nil {
		t.Fatal("eleventh conversation accepted")
	}
	if _, err := a.Chat("", "duplicate", "request-0"); err == nil {
		t.Fatal("duplicate request accepted")
	}
	a.mu.Lock()
	conversationID := a.active["request-0"].conversationID
	a.mu.Unlock()
	if _, err := a.Chat(conversationID, "overlap", "overlap"); err == nil {
		t.Fatal("same conversation overlapped")
	}
	if err := a.DeleteConversation(conversationID); err == nil {
		t.Fatal("running conversation deleted")
	}
	idle, _, _, err := a.store.Begin(context.Background(), "", "idle")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteConversation(idle.ID); err != nil {
		t.Fatal("unrelated conversation could not be deleted", err)
	}
	if _, err := a.SaveSettings(SettingsInput{}); err == nil {
		t.Fatal("settings changed during answers")
	}
	if done, err := a.beginSourceChange("qq"); err == nil {
		done()
		t.Fatal("source changed during answers")
	}
	if done, err := a.beginCampusChange(); err == nil {
		done()
		t.Fatal("campus changed during answers")
	}
	a.Cancel("request-0")
	select {
	case got := <-finished:
		if got.err != nil || got.id != "request-0" || got.result.Message.State != "cancelled" || got.result.Message.Content != "回答：request-0" {
			t.Fatalf("wrong cancellation result: %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not finish")
	}
	a.mu.Lock()
	remaining := len(a.active)
	a.mu.Unlock()
	if remaining != 9 {
		t.Fatalf("cancel removed other requests: %d remain", remaining)
	}
	start("replacement")
	select {
	case id := <-entered:
		if id != "replacement" {
			t.Fatal(id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("released slot not reusable")
	}
	a.Cancel("request-0") // A late cancellation must not reach the replacement.
	close(release)
	for i := 0; i < maxConcurrentConversations; i++ {
		select {
		case got := <-finished:
			if got.err != nil || got.result.Message.State != "complete" || got.result.Message.Content != "完成："+got.id {
				t.Fatalf("responses mixed: %+v", got)
			}
			messages, err := a.GetMessages(got.result.Conversation.ID)
			if err != nil || len(messages) != 2 || messages[0].Content != got.id || messages[1].Content != "完成："+got.id {
				t.Fatalf("history mixed for %s", got.id)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("remaining answers did not finish")
		}
	}
}

func TestShutdownCancelsAndWaitsForEveryConversation(t *testing.T) {
	a := testApp(t)
	entered := make(chan struct{}, 2)
	finished := make(chan error, 2)
	a.answer = func(ctx context.Context, _ config.Model, _ []storage.Message, emit func(agent.Event)) (agent.Result, error) {
		emit(agent.Event{Kind: "delta", Text: "已显示"})
		entered <- struct{}{}
		<-ctx.Done()
		return agent.Result{}, ctx.Err()
	}
	for _, id := range []string{"a", "b"} {
		go func(id string) {
			r, err := a.Chat("", id, id)
			if err == nil && (r.Message.State != "cancelled" || r.Message.Content != "已显示") {
				err = fmt.Errorf("lost answer for %s", id)
			}
			finished <- err
		}(id)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("answer did not start")
		}
	}
	a.shutdown(context.Background())
	for i := 0; i < 2; i++ {
		select {
		case err := <-finished:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("shutdown did not wait")
		}
	}
	if _, err := a.Chat("", "late", "late"); err == nil {
		t.Fatal("chat accepted after shutdown")
	}
	if _, err := a.ListConversations(); err == nil {
		t.Fatal("store remained open")
	}
}

func TestFailedConversationStartReleasesItsSlot(t *testing.T) {
	a := testApp(t)
	if _, err := a.Chat("missing-conversation", "问题", "failed"); err == nil {
		t.Fatal("unknown conversation accepted")
	}
	a.mu.Lock()
	remaining := len(a.active)
	a.mu.Unlock()
	if remaining != 0 {
		t.Fatal("failed start retained its slot")
	}
	a.answer = func(context.Context, config.Model, []storage.Message, func(agent.Event)) (agent.Result, error) {
		return agent.Result{Text: "完成"}, nil
	}
	if result, err := a.Chat("", "新问题", "new"); err != nil || result.Message.State != "complete" {
		t.Fatal("chat could not start after failed save", err)
	}
}
