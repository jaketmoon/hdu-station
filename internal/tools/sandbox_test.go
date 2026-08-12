package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	stationsandbox "github.com/jaketmoon/hdu-station/internal/sandbox"
)

type fakeSandboxExecutor struct {
	status  stationsandbox.Status
	started bool
	called  bool
	request stationsandbox.Request
}

func (executor *fakeSandboxExecutor) Status(context.Context) stationsandbox.Status {
	return executor.status
}

func (executor *fakeSandboxExecutor) Execute(_ context.Context, request stationsandbox.Request) (stationsandbox.Result, error) {
	executor.called = true
	executor.request = request
	return stationsandbox.Result{Stdout: "inside guest", ExitCode: 0}, nil
}

func (executor *fakeSandboxExecutor) Install(context.Context) error { return nil }

func (executor *fakeSandboxExecutor) Start(context.Context) error {
	executor.started = true
	executor.status.Ready = true
	return nil
}

func (executor *fakeSandboxExecutor) Stop(context.Context) error { return nil }

func (executor *fakeSandboxExecutor) Purge(context.Context) error { return nil }

func TestSandboxExecuteToolStartsOnlyInjectedLifecycleAndReturnsBoundedResult(t *testing.T) {
	executor := &fakeSandboxExecutor{status: stationsandbox.Status{Supported: true, Reason: "not started"}}
	tool := NewSandboxExecuteTool(executor)
	if tool.ReadOnly() {
		t.Fatal("sandbox execution must not be registered as read-only")
	}
	registry, err := NewRegistry(tool)
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Call(context.Background(), tool.Definition().Name, json.RawMessage(`{"command":"python","args":["-c","print(1)"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !executor.started || !executor.called || executor.request.Command != "python" || len(executor.request.Args) != 2 {
		t.Fatalf("sandbox lifecycle/request not used: started=%v called=%v request=%#v", executor.started, executor.called, executor.request)
	}
	if !strings.Contains(result.Text, "inside guest") {
		t.Fatalf("unexpected Sandbox result: %s", result.Text)
	}
	if _, err := NewReadOnlyRegistry(tool); err == nil || !strings.Contains(err.Error(), "not read-only") {
		t.Fatalf("read-only registry accepted Sandbox execution: %v", err)
	}
}

func TestSandboxExecuteToolDoesNotFallbackWhenUnavailable(t *testing.T) {
	executor := &unavailableSandboxExecutor{}
	tool := NewSandboxExecuteTool(executor)
	_, err := tool.Call(context.Background(), json.RawMessage(`{"command":"echo"}`))
	if err == nil || !errors.Is(err, stationsandbox.ErrUnavailable) {
		t.Fatalf("unexpected unavailable Sandbox error: %v", err)
	}
}

func TestSandboxExecuteToolRejectsNulBeforeStartingGuest(t *testing.T) {
	executor := &fakeSandboxExecutor{status: stationsandbox.Status{Supported: true, Ready: true}}
	tool := NewSandboxExecuteTool(executor)
	for _, arguments := range []string{
		`{"command":"echo\u0000bad"}`,
		`{"command":"echo","args":["ok\u0000bad"]}`,
	} {
		if _, err := tool.Call(context.Background(), json.RawMessage(arguments)); err == nil || !strings.Contains(err.Error(), "NUL") {
			t.Fatalf("NUL-containing request was accepted: %s, err=%v", arguments, err)
		}
	}
	if executor.called || executor.started {
		t.Fatal("NUL-containing request reached the Sandbox lifecycle")
	}
}

type unavailableSandboxExecutor struct{}

func (*unavailableSandboxExecutor) Status(context.Context) stationsandbox.Status {
	return stationsandbox.Status{Reason: "platform unavailable"}
}

func (*unavailableSandboxExecutor) Execute(context.Context, stationsandbox.Request) (stationsandbox.Result, error) {
	return stationsandbox.Result{}, errors.New("must not execute")
}
