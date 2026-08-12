package sandbox

import (
	"context"
	"errors"
	"fmt"
	runtimepkg "runtime"
	"strings"
)

var ErrUnavailable = errors.New("sandbox is unavailable")

type Status struct {
	Platform  string `json:"platform"`
	Supported bool   `json:"supported"`
	Ready     bool   `json:"ready"`
	Reason    string `json:"reason"`
}

type Request struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

type Result struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

type Executor interface {
	Status(context.Context) Status
	Execute(context.Context, Request) (Result, error)
}

// Lifecycle is implemented by platform backends that can install and manage
// the Station-owned guest. It is intentionally separate from Executor so a
// backend can be inspected and tested without making installation a hidden
// side effect of the first command.
type Lifecycle interface {
	Executor
	Install(context.Context) error
	Start(context.Context) error
	Stop(context.Context) error
	Purge(context.Context) error
}

type unavailableExecutor struct {
	status Status
}

func NewCurrent() Executor {
	return NewCurrentAt("")
}

func New(goos, goarch string) Executor {
	return NewAt("", goos, goarch)
}

func NewCurrentAt(dataRoot string) Executor {
	return NewAtWithImage(dataRoot, runtimepkg.GOOS, runtimepkg.GOARCH, ImageSource{})
}

func NewAt(dataRoot, goos, goarch string) Executor {
	return NewAtWithImage(dataRoot, goos, goarch, ImageSource{})
}

func NewCurrentAtWithImage(dataRoot string, image ImageSource) Executor {
	return NewAtWithImage(dataRoot, runtimepkg.GOOS, runtimepkg.GOARCH, image)
}

func NewAtWithImage(dataRoot, goos, goarch string, image ImageSource) Executor {
	return newPlatformExecutor(dataRoot, goos, goarch, image, Detect(goos, goarch))
}

func Detect(goos, goarch string) Status {
	platform := goos + "/" + goarch
	switch {
	case goos == "darwin" && goarch == "arm64":
		return Status{
			Platform:  platform,
			Supported: true,
			Reason:    "macOS Virtualization.framework backend is not installed",
		}
	case goos == "windows" && goarch == "amd64":
		return Status{
			Platform:  platform,
			Supported: true,
			Reason:    "Windows WSL2 backend is not installed",
		}
	default:
		return Status{
			Platform: platform,
			Reason:   "this platform is not supported by HDU Station Sandbox",
		}
	}
}

func (e *unavailableExecutor) Status(context.Context) Status {
	return e.status
}

func (e *unavailableExecutor) Execute(_ context.Context, request Request) (Result, error) {
	if request.Command == "" {
		return Result{}, errors.New("sandbox command is required")
	}
	return Result{}, fmt.Errorf("%w: %s", ErrUnavailable, e.status.Reason)
}

func (e *unavailableExecutor) Install(context.Context) error { return e.unavailableError() }

func (e *unavailableExecutor) Start(context.Context) error { return e.unavailableError() }

func (e *unavailableExecutor) Stop(context.Context) error { return e.unavailableError() }

func (e *unavailableExecutor) Purge(context.Context) error { return e.unavailableError() }

func (e *unavailableExecutor) unavailableError() error {
	return fmt.Errorf("%w: %s", ErrUnavailable, e.status.Reason)
}

func validateRequest(request Request) error {
	if request.Command == "" {
		return errors.New("sandbox command is required")
	}
	if strings.ContainsRune(request.Command, '\x00') {
		return errors.New("sandbox command contains a NUL byte")
	}
	for _, argument := range request.Args {
		if strings.ContainsRune(argument, '\x00') {
			return errors.New("sandbox argument contains a NUL byte")
		}
	}
	return nil
}
