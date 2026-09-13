package main

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaketmoon/hdu-station/internal/tools"
)

// Explicit settings action: preserve the logged-in account and resolve the
// search challenge instead of deleting cookies and requesting another login QR.
func (a *App) BeginSourceVerification() (SourceLogin, error) {
	done, err := a.beginSourceChange("xiaohongshu")
	if err != nil {
		return SourceLogin{}, err
	}
	defer done()
	a.cancelSourceLogin("xiaohongshu")
	a.mu.Lock()
	cfg := a.cfg.Sources
	a.mu.Unlock()
	cfg.Xiaohongshu.Enabled = true
	ctx, cancel := context.WithTimeout(a.ctx, 75*time.Second)
	defer cancel()
	r, err := tools.NewXiaohongshuClient(cfg.Xiaohongshu, a.root).BeginVerification(ctx)
	if err != nil {
		return SourceLogin{}, err
	}
	id := uuid.NewString()
	result := SourceLogin{ID: id, LoginChallenge: tools.LoginChallenge{Kind: "verification", Status: r.Status, Image: r.Image, ExpiresAt: r.ExpiresAt}}
	if r.Status != "waiting" {
		return result, nil
	}
	loginCtx, loginCancel := context.WithDeadline(a.ctx, time.UnixMilli(r.ExpiresAt))
	session := &sourceLoginSession{id: id, source: "xiaohongshu", verificationID: r.ID, status: "waiting", cfg: cfg, ctx: loginCtx, cancel: loginCancel, expiresAt: r.ExpiresAt}
	a.mu.Lock()
	if a.logins == nil {
		a.logins = map[string]*sourceLoginSession{}
	}
	a.logins[id] = session
	a.mu.Unlock()
	return result, nil
}
func (a *App) cancelVerification(login *sourceLoginSession) {
	if login.verificationID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tools.NewXiaohongshuClient(login.cfg.Xiaohongshu, a.root).CancelVerification(ctx, login.verificationID)
}
