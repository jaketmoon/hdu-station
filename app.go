package main

import (
	"context"
	"runtime"

	"github.com/jaketmoon/hdu-station/internal/config"
)

type App struct {
	ctx      context.Context
	dataRoot string
	config   config.Config
}

func NewApp(dataRoot string, cfg config.Config) *App {
	return &App{dataRoot: dataRoot, config: cfg}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

type BootstrapState struct {
	Name          string              `json:"name"`
	Version       string              `json:"version"`
	Platform      string              `json:"platform"`
	DataRoot      string              `json:"dataRoot"`
	Configuration ConfigurationStatus `json:"configuration"`
}

type ConfigurationStatus struct {
	CampusConfigured    bool     `json:"campusConfigured"`
	DefaultProvider     string   `json:"defaultProvider"`
	ConfiguredProviders []string `json:"configuredProviders"`
}

func (a *App) Bootstrap() BootstrapState {
	status := a.config.Status()
	return BootstrapState{
		Name:     "HDU Station",
		Version:  "0.1.0-dev",
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
		DataRoot: a.dataRoot,
		Configuration: ConfigurationStatus{
			CampusConfigured:    status.CampusConfigured,
			DefaultProvider:     status.DefaultProvider,
			ConfiguredProviders: status.ConfiguredProviders,
		},
	}
}
