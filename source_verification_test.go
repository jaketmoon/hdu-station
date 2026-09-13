package main

import (
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

func TestSecurityVerificationPreservesLoginAndRequiresSearchRecovery(t *testing.T) {
	var recovered, cancelled atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer service-secret" {
			t.Error("service authorization lost")
		}
		switch r.URL.Path {
		case "/api/v1/login/status":
			fmt.Fprint(w, `{"success":true,"data":{"is_logged_in":true,"verification_required":true}}`)
		case "/api/v1/security/qrcode":
			fmt.Fprintf(w, `{"success":true,"data":{"id":"0123456789abcdef0123456789abcdef","status":"waiting","expiresAt":%d,"image":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jO1sAAAAASUVORK5CYII="}}`, time.Now().Add(45*time.Second).UnixMilli())
		case "/api/v1/security/poll":
			status := "waiting"
			if recovered.Load() {
				status = "ready"
			}
			fmt.Fprintf(w, `{"success":true,"data":{"status":%q}}`, status)
		case "/api/v1/security/cancel":
			cancelled.Store(true)
			fmt.Fprint(w, `{"success":true,"data":{}}`)
		default:
			t.Error("verification must not delete cookies or request a login QR")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	a := testApp(t)
	a.cfg.Sources.Xiaohongshu = config.Xiaohongshu{Enabled: true, BaseURL: server.URL, AuthToken: "service-secret"}
	state, err := a.CheckSource("xiaohongshu")
	if err != nil || state.Status != "verification_required" {
		t.Fatal("logged-in security challenge misclassified", state, err)
	}
	r, err := a.BeginSourceVerification()
	if err != nil || r.Kind != "verification" || r.Status != "waiting" {
		t.Fatal("verification not started", err)
	}
	encoded, _ := json.Marshal(r)
	if strings.Contains(string(encoded), "service-secret") || strings.Contains(string(encoded), "0123456789abcdef") {
		t.Fatal("private service state exposed")
	}
	p, err := a.PollSourceLogin(r.ID)
	if err != nil || p.Status != "waiting" {
		t.Fatal("login was mistaken for search recovery")
	}
	recovered.Store(true)
	p, err = a.PollSourceLogin(r.ID)
	if err != nil || p.Status != "ready" {
		t.Fatal("confirmed recovery not reported")
	}
	a.CancelSourceLogin(r.ID)
	if !cancelled.Load() {
		t.Fatal("service verification was not cleaned up")
	}
	p, _ = a.PollSourceLogin(r.ID)
	if p.Status != "cancelled" {
		t.Fatal("stale verification reused")
	}
}

func TestSecurityVerificationUnsupportedService(t *testing.T) {
	s := httptest.NewServer(http.NotFoundHandler())
	defer s.Close()
	a := testApp(t)
	a.cfg.Sources.Xiaohongshu = config.Xiaohongshu{Enabled: true, BaseURL: s.URL}
	_, err := a.BeginSourceVerification()
	if err == nil || !strings.Contains(err.Error(), "更新连接组件") {
		t.Fatal("old service needs an actionable unsupported message", err)
	}
}
