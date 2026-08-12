package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	stationsandbox "github.com/jaketmoon/hdu-station/internal/sandbox"
)

const (
	maxSandboxCommandBytes  = 256
	maxSandboxArguments     = 64
	maxSandboxArgumentBytes = 4096
	maxSandboxToolOutput    = 1 << 20
)

// SandboxExecuteTool is deliberately not read-only: it runs user-requested
// commands, but only through the injected sandbox.Executor. It has no command
// runner or host-process fallback by design.
type SandboxExecuteTool struct {
	executor stationsandbox.Executor
}

func NewSandboxExecuteTool(executor stationsandbox.Executor) *SandboxExecuteTool {
	return &SandboxExecuteTool{executor: executor}
}

func (tool *SandboxExecuteTool) Definition() Definition {
	return Definition{
		Name:        "sandbox_execute",
		Description: "Execute a command inside the HDU Station isolated Sandbox. The command never runs on the desktop host.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","minLength":1,"maxLength":256},"args":{"type":"array","maxItems":64,"items":{"type":"string","maxLength":4096}}},"required":["command"],"additionalProperties":false}`),
	}
}

func (tool *SandboxExecuteTool) ReadOnly() bool { return false }

func (tool *SandboxExecuteTool) Call(ctx context.Context, arguments json.RawMessage) (Result, error) {
	if tool == nil || tool.executor == nil {
		return Result{}, errors.New("HDU Station Sandbox executor is not initialized")
	}
	var input struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode Sandbox execution arguments: %w", err)
	}
	input.Command = strings.TrimSpace(input.Command)
	if input.Command == "" || strings.ContainsRune(input.Command, '\x00') || len([]byte(input.Command)) > maxSandboxCommandBytes {
		return Result{}, errors.New("Sandbox command must be non-empty, contain no NUL bytes, and be at most 256 bytes")
	}
	if len(input.Args) > maxSandboxArguments {
		return Result{}, errors.New("Sandbox supports at most 64 arguments")
	}
	for _, argument := range input.Args {
		if strings.ContainsRune(argument, '\x00') || len([]byte(argument)) > maxSandboxArgumentBytes {
			return Result{}, errors.New("Sandbox arguments must contain no NUL bytes and be at most 4096 bytes each")
		}
	}

	status := tool.executor.Status(ctx)
	if !status.Ready {
		lifecycle, ok := tool.executor.(stationsandbox.Lifecycle)
		if !ok {
			return Result{}, fmt.Errorf("%w: %s", stationsandbox.ErrUnavailable, status.Reason)
		}
		if err := lifecycle.Start(ctx); err != nil {
			return Result{}, fmt.Errorf("start HDU Station Sandbox: %w", err)
		}
	}
	result, err := tool.executor.Execute(ctx, stationsandbox.Request{Command: input.Command, Args: input.Args})
	if err != nil {
		return Result{}, fmt.Errorf("execute command in HDU Station Sandbox: %w", err)
	}
	encoded, err := json.Marshal(struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode int    `json:"exit_code"`
	}{
		Stdout:   truncateSandboxOutput(result.Stdout),
		Stderr:   truncateSandboxOutput(result.Stderr),
		ExitCode: result.ExitCode,
	})
	if err != nil {
		return Result{}, fmt.Errorf("encode Sandbox result: %w", err)
	}
	return Result{Text: string(encoded)}, nil
}

func truncateSandboxOutput(output string) string {
	if len([]byte(output)) <= maxSandboxToolOutput {
		return output
	}
	return string([]byte(output)[:maxSandboxToolOutput]) + "\n[output truncated by Station]"
}
