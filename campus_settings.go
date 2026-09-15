package main

import (
	"context"
	"errors"
	"runtime"
	"time"

	"github.com/jaketmoon/hdu-station/internal/campusauth"
)

type CampusConnection struct {
	HasCredential  bool   `json:"hasCredential"`
	Status         string `json:"status"`
	Message        string `json:"message,omitempty"`
	ScheduleAccess bool   `json:"scheduleAccess"`
}

func (a *App) campusConnection() CampusConnection {
	connection := CampusConnection{HasCredential: a.campus.Configured(), ScheduleAccess: a.campus.HasScope(campusauth.ScheduleScope)}
	if !campusauth.Supported(runtime.GOOS) {
		connection.Status = "unsupported"
		return connection
	}
	connection.Status = a.campus.Status()
	return connection
}

func (a *App) CheckCampus() CampusConnection {
	return a.campusConnection()
}

func (a *App) beginCampusChange() (func(), error) {
	a.sourceMu.Lock()
	a.mu.Lock()
	if len(a.active) > 0 {
		a.mu.Unlock()
		a.sourceMu.Unlock()
		return nil, errors.New("请等当前回答结束后再修改校园连接")
	}
	a.sourceBusy = true
	a.mu.Unlock()
	return func() { a.mu.Lock(); a.sourceBusy = false; a.mu.Unlock(); a.sourceMu.Unlock() }, nil
}

// These account actions belong exclusively to settings, never Agent tools.
func (a *App) BeginCampusLogin() (campusauth.Login, error) {
	if !campusauth.Supported(runtime.GOOS) {
		return campusauth.Login{}, errors.New("当前平台暂不支持校园网页授权")
	}
	done, err := a.beginCampusChange()
	if err != nil {
		return campusauth.Login{}, err
	}
	defer done()
	return a.campus.Begin(a.ctx, a.openCampusBrowser)
}

func (a *App) PollCampusLogin(id string) campusauth.Login { return a.campus.Poll(id) }
func (a *App) CancelCampusLogin(id string) campusauth.Login {
	a.campus.Cancel(id)
	return a.campus.Poll(id)
}
func (a *App) OpenCampusLogin(id string) error { return a.campus.Open(id, a.openCampusBrowser) }

func (a *App) LogoutCampus() (CampusConnection, error) {
	done, err := a.beginCampusChange()
	if err != nil {
		return CampusConnection{}, err
	}
	defer done()
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	message, err := a.campus.Logout(ctx)
	if err != nil {
		return CampusConnection{}, err
	}
	return CampusConnection{Status: "logged_out", Message: message}, nil
}
