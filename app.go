package main

import (
	"context"
	"runtime"
)

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

type BootstrapState struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
}

func (a *App) Bootstrap() BootstrapState {
	return BootstrapState{
		Name:     "HDU Station",
		Version:  "0.1.0-dev",
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}
}
