package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jaketmoon/hdu-station/internal/config"
)

func TestSourceLoginPersistsConfigurationAndTracksSavedCookies(t *testing.T) {
	var loggedIn atomic.Bool
	var checks atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer service-private-sentinel" {
			t.Error("existing service auth not preserved")
		}
		switch r.URL.Path {
		case "/api/v1/login/cookies":
			if r.Method != http.MethodDelete {
				t.Error("wrong clear method")
			}
			loggedIn.Store(false)
			fmt.Fprint(w, `{"success":true,"data":{}}`)
		case "/api/v1/login/qrcode":
			fmt.Fprint(w, `{"success":true,"data":{"is_logged_in":false,"timeout":"4m0s","img":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jO1sAAAAASUVORK5CYII="}}`)
		case "/api/v1/login/status":
			checks.Add(1)
			fmt.Fprintf(w, `{"success":true,"data":{"is_logged_in":%t,"cookie":"private-sentinel"}}`, loggedIn.Load())
		default:
			t.Error("unexpected endpoint")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	a := testApp(t)
	a.cfg.Sources.Xiaohongshu = config.Xiaohongshu{BaseURL: server.URL, AuthToken: "service-private-sentinel"}
	if _, err := a.SetSourceEnabled("xiaohongshu", true); err != nil {
		t.Fatal(err)
	}
	login, err := a.BeginSourceLogin("xiaohongshu")
	if err != nil || login.ID == "" || login.Status != "waiting" {
		t.Fatal("login not started")
	}
	state, err := a.PollSourceLogin(login.ID)
	if err != nil || state.Status != "waiting" {
		t.Fatal("unscanned QR treated as authorized")
	}
	loggedIn.Store(true) // The external service has now saved cookies after scanning.
	state, err = a.PollSourceLogin(login.ID)
	if err != nil || state.Status != "ready" {
		t.Fatal("saved cookie not detected")
	}
	data, _ := json.Marshal(state)
	if strings.Contains(string(data), "private-sentinel") {
		t.Fatal("login leaked credentials")
	}
	a.CancelSourceLogin(login.ID)
	state, _ = a.PollSourceLogin(login.ID)
	if state.Status != "cancelled" {
		t.Fatal("cancelled login reused")
	}
	result, err := a.ClearSourceCredentials("xiaohongshu")
	if err != nil || result.Status != "logged_out" || loggedIn.Load() {
		t.Fatal("cookie not cleared")
	}
	stored, _ := config.Load(a.root)
	if stored.Sources.Xiaohongshu.AuthToken != "service-private-sentinel" || stored.Sources.Xiaohongshu.BaseURL != server.URL {
		t.Fatal("clear destroyed connection configuration")
	}
	before := checks.Load()
	if _, err := a.SetSourceEnabled("xiaohongshu", false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetSourceEnabled("qq", false); err != nil {
		t.Fatal(err)
	}
	if checks.Load() != before {
		t.Fatal("disabled source made a request")
	}
	stored, _ = config.Load(a.root)
	if !stored.Sources.QQ.Disabled || stored.Sources.Xiaohongshu.Enabled {
		t.Fatal("source toggles not persisted")
	}
}
func TestSourceLoginRejectsUnknownSourceAndChangesDuringChat(t *testing.T) {
	a := testApp(t)
	for _, source := range []string{"zanao", "shell", "http://example.test"} {
		if _, err := a.BeginSourceLogin(source); err == nil {
			t.Fatal("untrusted login source accepted")
		}
		if _, err := a.ClearSourceCredentials(source); err == nil {
			t.Fatal("untrusted clear source accepted")
		}
		if _, err := a.SetSourceEnabled(source, true); err == nil {
			t.Fatal("untrusted source setting accepted")
		}
	}
	a.active = &activeTurn{cancel: func() {}, done: make(chan struct{})}
	close(a.active.done)
	if _, err := a.BeginSourceLogin("xiaohongshu"); err == nil {
		t.Fatal("login during chat accepted")
	}
	if _, err := a.ClearSourceCredentials("qq"); err == nil {
		t.Fatal("logout during chat accepted")
	}
	a.active = nil
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	a.logins = map[string]*sourceLoginSession{"expired": {id: "expired", source: "qq", status: "waiting", ctx: ctx, cancel: cancel}}
	if state, err := a.PollSourceLogin("expired"); err != nil || state.Status != "expired" {
		t.Fatal("expired QR polled upstream")
	}
}
func TestCancelledLoginCannotReplaceVisibleState(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	a := testApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	a.logins = map[string]*sourceLoginSession{"old": {id: "old", source: "xiaohongshu", status: "waiting", ctx: ctx, cancel: cancel, cfg: config.Sources{Xiaohongshu: config.Xiaohongshu{Enabled: true, BaseURL: server.URL}}}}
	finished := make(chan SourceLogin, 1)
	go func() { state, _ := a.PollSourceLogin("old"); finished <- state }()
	<-entered
	a.CancelSourceLogin("old")
	select {
	case state := <-finished:
		if state.Status != "cancelled" {
			t.Fatal("stale poll completed login")
		}
	case <-time.After(time.Second):
		t.Fatal("login cancellation did not cancel HTTP request")
	}
}
