package main

import (
	"context"
	"log"
	"runtime"
	"sync"

	"github.com/jaketmoon/hdu-station/internal/config"
)

type App struct {
	mu                  sync.RWMutex
	ctx                 context.Context
	dataRoot            string
	config              config.Config
	initializationError string
	initialize          func() (*App, error)
}

func NewApp(dataRoot string, cfg config.Config) *App {
	return &App{dataRoot: dataRoot, config: cfg}
}

func newRuntimeApp(initialize func() (*App, error)) *App {
	return &App{config: config.Default(), initialize: initialize}
}

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	initialize := a.initialize
	a.mu.Unlock()

	if initialize == nil {
		return
	}

	initialized, err := initialize()
	if err != nil {
		log.Printf("initialize HDU Station: %v", err)
		a.mu.Lock()
		a.initializationError = err.Error()
		a.mu.Unlock()
		return
	}

	a.mu.Lock()
	a.dataRoot = initialized.dataRoot
	a.config = initialized.config
	a.initialize = nil
	a.mu.Unlock()
}

type BootstrapState struct {
	Name          string              `json:"name"`
	Version       string              `json:"version"`
	Platform      string              `json:"platform"`
	DataRoot      string              `json:"dataRoot"`
	StartupError  string              `json:"startupError,omitempty"`
	Configuration ConfigurationStatus `json:"configuration"`
}

type ConfigurationStatus struct {
	CampusConfigured    bool     `json:"campusConfigured"`
	DefaultProvider     string   `json:"defaultProvider"`
	ConfiguredProviders []string `json:"configuredProviders"`
}

func (a *App) Bootstrap() BootstrapState {
	a.mu.RLock()
	defer a.mu.RUnlock()

	status := a.config.Status()
	return BootstrapState{
		Name:         "HDU Station",
		Version:      "0.1.0-dev",
		Platform:     runtime.GOOS + "/" + runtime.GOARCH,
		DataRoot:     a.dataRoot,
		StartupError: a.initializationError,
		Configuration: ConfigurationStatus{
			CampusConfigured:    status.CampusConfigured,
			DefaultProvider:     status.DefaultProvider,
			ConfiguredProviders: status.ConfiguredProviders,
		},
	}
}
