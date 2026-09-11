package main

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/jaketmoon/hdu-station/internal/tools"
)

type SourceConnection struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
}
type SourceLogin struct {
	ID string `json:"id"`
	tools.LoginChallenge
}
type sourceLoginSession struct {
	id, source, status string
	cfg                config.Sources
	ctx                context.Context
	cancel             context.CancelFunc
	expiresAt          int64
	poll               sync.Mutex
}

func validLoginSource(source string) bool { return source == "qq" || source == "xiaohongshu" }

// Account mutations are desktop settings actions, never model tools. Serialize
// mutations without holding the application lock over network or CLI calls.
func (a *App) beginSourceChange(source string) (func(), error) {
	if !validLoginSource(source) {
		return nil, errors.New("不支持这个登录来源")
	}
	a.sourceMu.Lock()
	a.mu.Lock()
	if a.active != nil {
		a.mu.Unlock()
		a.sourceMu.Unlock()
		return nil, errors.New("请等当前回答结束后再修改来源连接")
	}
	a.sourceBusy = true
	a.mu.Unlock()
	return func() { a.mu.Lock(); a.sourceBusy = false; a.mu.Unlock(); a.sourceMu.Unlock() }, nil
}
func (a *App) sourceConnection(ctx context.Context, source string, cfg config.Sources) SourceConnection {
	if source == "qq" {
		if cfg.QQ.Disabled {
			return SourceConnection{Status: "disabled"}
		}
		return SourceConnection{Enabled: true, Status: a.client.Status(ctx)}
	}
	return SourceConnection{Enabled: cfg.Xiaohongshu.Enabled, Status: tools.NewXiaohongshuClient(cfg.Xiaohongshu, a.root).Status(ctx)}
}
func (a *App) CheckSource(source string) (SourceConnection, error) {
	if !validLoginSource(source) {
		return SourceConnection{}, errors.New("不支持这个登录来源")
	}
	a.mu.Lock()
	cfg := a.cfg.Sources
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(a.ctx, 25*time.Second)
	defer cancel()
	return a.sourceConnection(ctx, source, cfg), nil
}
func (a *App) cancelSourceLogin(source string) {
	a.mu.Lock()
	var cancelled []*sourceLoginSession
	for id, login := range a.logins {
		if login.source == source {
			login.cancel()
			delete(a.logins, id)
			cancelled = append(cancelled, login)
		}
	}
	a.mu.Unlock()
	// Wait for a cancelled CLI poll to exit before clearing or replacing its
	// saved token. Otherwise a late authorization could restore a cleared login.
	for _, login := range cancelled {
		login.poll.Lock()
		login.poll.Unlock()
	}
}
func (a *App) SetSourceEnabled(source string, enabled bool) (SourceConnection, error) {
	done, err := a.beginSourceChange(source)
	if err != nil {
		return SourceConnection{}, err
	}
	defer done()
	a.cancelSourceLogin(source)
	a.mu.Lock()
	next := a.cfg
	if source == "qq" {
		next.Sources.QQ.Disabled = !enabled
	} else {
		next.Sources.Xiaohongshu.Enabled = enabled
	}
	err = config.Save(a.root, next)
	if err == nil {
		a.cfg = next
	}
	a.mu.Unlock()
	if err != nil {
		return SourceConnection{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 25*time.Second)
	defer cancel()
	return a.sourceConnection(ctx, source, next.Sources), nil
}
func (a *App) BeginSourceLogin(source string) (SourceLogin, error) {
	done, err := a.beginSourceChange(source)
	if err != nil {
		return SourceLogin{}, err
	}
	defer done()
	a.cancelSourceLogin(source)
	a.mu.Lock()
	cfg := a.cfg.Sources
	a.mu.Unlock()
	// Connecting an account does not implicitly enable a search source.
	cfg.Xiaohongshu.Enabled = true
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	var challenge tools.LoginChallenge
	if source == "qq" {
		if _, err = tools.EnsureTencentCLI(ctx, a.root, runtime.GOOS, runtime.GOARCH, nil); err != nil {
			return SourceLogin{}, errors.New("QQ 连接组件暂不可用，请检查网络或平台支持")
		}
		challenge, err = a.client.BeginLogin(ctx)
	} else {
		client := tools.NewXiaohongshuClient(cfg.Xiaohongshu, a.root)
		// Reconnect explicitly replaces the account session and produces a new QR.
		if err = client.ClearCredentials(ctx); err == nil {
			challenge, err = client.BeginLogin(ctx)
		}
	}
	if err != nil {
		return SourceLogin{}, err
	}
	id := uuid.NewString()
	if challenge.Status != "waiting" {
		return SourceLogin{ID: id, LoginChallenge: challenge}, nil
	}
	loginCtx, loginCancel := context.WithDeadline(a.ctx, time.UnixMilli(challenge.ExpiresAt))
	session := &sourceLoginSession{id: id, source: source, status: "waiting", cfg: cfg, ctx: loginCtx, cancel: loginCancel, expiresAt: challenge.ExpiresAt}
	a.mu.Lock()
	if a.logins == nil {
		a.logins = map[string]*sourceLoginSession{}
	}
	a.logins[id] = session
	a.mu.Unlock()
	return SourceLogin{ID: id, LoginChallenge: challenge}, nil
}
func (a *App) PollSourceLogin(id string) (SourceLogin, error) {
	a.mu.Lock()
	login := a.logins[id]
	a.mu.Unlock()
	if login == nil {
		return SourceLogin{ID: id, LoginChallenge: tools.LoginChallenge{Status: "cancelled"}}, nil
	}
	login.poll.Lock()
	defer login.poll.Unlock()
	result := func(status string) SourceLogin {
		return SourceLogin{ID: id, LoginChallenge: tools.LoginChallenge{Status: status, ExpiresAt: login.expiresAt}}
	}
	if login.status != "waiting" {
		return result(login.status), nil
	}
	if login.ctx.Err() != nil {
		if errors.Is(login.ctx.Err(), context.DeadlineExceeded) {
			return result("expired"), nil
		}
		return result("cancelled"), nil
	}
	var status string
	var err error
	if login.source == "qq" {
		status, err = a.client.CompleteLogin(login.ctx)
	} else {
		ctx, cancel := context.WithTimeout(login.ctx, 25*time.Second)
		status = tools.NewXiaohongshuClient(login.cfg.Xiaohongshu, a.root).Status(ctx)
		cancel()
		switch status {
		case "logged_out", "unavailable":
			status = "waiting"
		case "auth_failed":
			err = errors.New("小红书连接验证失败，请检查本机连接组件")
		}
	}
	// Closing, replacing or clearing a session must invalidate late completions.
	if login.ctx.Err() != nil {
		if errors.Is(login.ctx.Err(), context.DeadlineExceeded) {
			return result("expired"), nil
		}
		return result("cancelled"), nil
	}
	if err != nil {
		login.status = "error"
		login.cancel()
		return SourceLogin{}, err
	}
	if status != "waiting" {
		login.status = status
		login.cancel()
	}
	return result(status), nil
}
func (a *App) CancelSourceLogin(id string) {
	a.sourceMu.Lock()
	defer a.sourceMu.Unlock()
	a.mu.Lock()
	login := a.logins[id]
	if login != nil {
		login.cancel()
		delete(a.logins, id)
	}
	a.mu.Unlock()
	if login != nil {
		login.poll.Lock()
		login.poll.Unlock()
	}
}
func (a *App) ClearSourceCredentials(source string) (SourceConnection, error) {
	done, err := a.beginSourceChange(source)
	if err != nil {
		return SourceConnection{}, err
	}
	defer done()
	a.cancelSourceLogin(source)
	a.mu.Lock()
	cfg := a.cfg.Sources
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(a.ctx, 25*time.Second)
	defer cancel()
	if source == "qq" {
		err = a.client.ClearCredentials(ctx)
	} else {
		cfg.Xiaohongshu.Enabled = true
		err = tools.NewXiaohongshuClient(cfg.Xiaohongshu, a.root).ClearCredentials(ctx)
	}
	if err != nil {
		return SourceConnection{}, err
	}
	a.mu.Lock()
	cfg = a.cfg.Sources
	a.mu.Unlock()
	enabled := !cfg.QQ.Disabled
	if source == "xiaohongshu" {
		enabled = cfg.Xiaohongshu.Enabled
	}
	status := "logged_out"
	if !enabled {
		status = "disabled"
	}
	return SourceConnection{Enabled: enabled, Status: status}, nil
}
