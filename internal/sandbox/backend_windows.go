//go:build windows

package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	wslExecutable       = "wsl.exe"
	stationDistribution = "HDU-Station-Sandbox"
	wslRuntimePath      = "/usr/local/bin/hdu-station-runtime"
	wslOwnershipFile    = "wsl-ownership.json"
	wslCommandTimeout   = 30 * time.Second
	wslRuntimeTimeout   = 11 * time.Minute
)

type wslOwnership struct {
	Version      int    `json:"version"`
	Distribution string `json:"distribution"`
}

type wslCommandRunner func(context.Context, string, ...string) ([]byte, error)
type wslRuntimeRunner func(context.Context, string, string, sandboxRuntimeRequest) (sandboxRuntimeResponse, error)

type wslExecutor struct {
	dataRoot     string
	image        ImageSource
	distribution string
	wsl          string
	run          wslCommandRunner
	runtime      wslRuntimeRunner
	statusMu     sync.RWMutex
	status       Status
}

func newPlatformExecutor(dataRoot, goos, goarch string, image ImageSource, status Status) Executor {
	if goos != "windows" || goarch != "amd64" {
		return &unavailableExecutor{status: status}
	}
	wsl, err := exec.LookPath(wslExecutable)
	if err != nil {
		status.Reason = "Windows WSL2 backend is not installed"
		return &unavailableExecutor{status: status}
	}
	executor := &wslExecutor{
		dataRoot:     dataRoot,
		image:        image,
		distribution: stationDistribution,
		wsl:          wsl,
		run:          runWSLCommand,
		runtime:      runWSLRuntime,
		status:       status,
	}
	executor.refreshStatus(context.Background())
	return executor
}

func runWSLCommand(ctx context.Context, binary string, arguments ...string) ([]byte, error) {
	commandContext, cancel := withDefaultTimeout(ctx, wslCommandTimeout)
	defer cancel()
	command := exec.CommandContext(commandContext, binary, arguments...)
	return command.Output()
}

func (e *wslExecutor) Status(ctx context.Context) Status {
	e.refreshStatus(ctx)
	e.statusMu.RLock()
	defer e.statusMu.RUnlock()
	return e.status
}

func (e *wslExecutor) Install(ctx context.Context) error {
	if e.dataRoot == "" {
		return errors.New("sandbox data root is required")
	}
	_, archive, err := e.loadImage(ctx)
	if err != nil {
		return err
	}
	exists, listErr := e.distributionExistsWithError(ctx)
	if listErr != nil {
		return fmt.Errorf("inspect WSL2 distributions before install: %w", listErr)
	}
	if exists {
		if !e.ownershipMarkerValid() {
			return errors.New("same-named WSL2 distribution exists but is not owned by HDU Station")
		}
		return errors.New("HDU Station WSL2 distribution is already installed")
	}
	if err := os.MkdirAll(filepath.Join(e.dataRoot, "sandbox"), 0o700); err != nil {
		return fmt.Errorf("create Station sandbox directory: %w", err)
	}
	if _, err := e.run(ctx, e.wsl, "--import", e.distribution, filepath.Join(e.dataRoot, "sandbox", "wsl"), archive, "--version", "2"); err != nil {
		return fmt.Errorf("import HDU Station WSL2 distribution: %w", err)
	}
	if err := e.hardenGuest(ctx); err != nil {
		_, _ = e.run(ctx, e.wsl, "--unregister", e.distribution)
		return err
	}
	if _, err := e.run(ctx, e.wsl, "--terminate", e.distribution); err != nil {
		_, _ = e.run(ctx, e.wsl, "--unregister", e.distribution)
		return fmt.Errorf("restart HDU Station WSL2 distribution after hardening: %w", err)
	}
	if err := e.writeOwnershipMarker(); err != nil {
		_, _ = e.run(ctx, e.wsl, "--unregister", e.distribution)
		return err
	}
	e.refreshStatus(ctx)
	return nil
}

func (e *wslExecutor) Start(ctx context.Context) error {
	owned, err := e.ownedDistributionExistsWithError(ctx)
	if err != nil {
		return fmt.Errorf("inspect HDU Station WSL2 distribution before start: %w", err)
	}
	if !owned {
		return fmt.Errorf("%w: WSL2 distribution is not installed", ErrUnavailable)
	}
	if e.runtime == nil {
		return errors.New("HDU Station guest runtime is not initialized")
	}
	if _, err := e.run(ctx, e.wsl, "--distribution", e.distribution, "--user", "root", "--exec", "/bin/true"); err != nil {
		return fmt.Errorf("start HDU Station WSL2 distribution: %w", err)
	}
	if response, err := e.runtime(ctx, e.wsl, e.distribution, sandboxRuntimeRequest{
		Version: sandboxRuntimeProtocolVersion,
		Op:      "ping",
	}); err != nil {
		return fmt.Errorf("check HDU Station guest runtime: %w", err)
	} else if response.Error != "" {
		return fmt.Errorf("check HDU Station guest runtime: %s", response.Error)
	}
	e.refreshStatus(ctx)
	return nil
}

func (e *wslExecutor) Stop(ctx context.Context) error {
	owned, err := e.ownedDistributionExistsWithError(ctx)
	if err != nil {
		return fmt.Errorf("inspect HDU Station WSL2 distribution before stop: %w", err)
	}
	if !owned {
		return nil
	}
	if _, err := e.run(ctx, e.wsl, "--terminate", e.distribution); err != nil {
		return fmt.Errorf("stop HDU Station WSL2 distribution: %w", err)
	}
	e.refreshStatus(ctx)
	return nil
}

func (e *wslExecutor) Purge(ctx context.Context) error {
	exists, listErr := e.distributionExistsWithError(ctx)
	if listErr != nil {
		return fmt.Errorf("inspect WSL2 distributions before purge: %w", listErr)
	}
	if !exists {
		// If the distro was removed outside Station, remove only our stale
		// marker. Leaving it behind could make a future same-named external
		// distro look owned after a restart.
		if err := e.removeOwnershipMarker(); err != nil {
			return err
		}
		e.refreshStatus(ctx)
		return nil
	}
	if !e.ownershipMarkerValid() {
		return nil
	}
	if err := e.Stop(ctx); err != nil {
		return err
	}
	if _, err := e.run(ctx, e.wsl, "--unregister", e.distribution); err != nil {
		return fmt.Errorf("purge HDU Station WSL2 distribution: %w", err)
	}
	if err := e.removeOwnershipMarker(); err != nil {
		return err
	}
	e.refreshStatus(ctx)
	return nil
}

func (e *wslExecutor) removeOwnershipMarker() error {
	if err := os.Remove(e.ownershipPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove HDU Station WSL2 ownership marker: %w", err)
	}
	return nil
}

func (e *wslExecutor) Execute(ctx context.Context, request Request) (Result, error) {
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	owned, err := e.ownedDistributionExistsWithError(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("inspect HDU Station WSL2 distribution before execute: %w", err)
	}
	if !owned {
		return Result{}, fmt.Errorf("%w: WSL2 distribution is not installed", ErrUnavailable)
	}
	if e.runtime == nil {
		return Result{}, errors.New("HDU Station guest runtime is not initialized")
	}
	response, err := e.runtime(ctx, e.wsl, e.distribution, sandboxRuntimeRequest{
		Version: sandboxRuntimeProtocolVersion,
		Op:      "execute",
		Command: request.Command,
		Args:    request.Args,
	})
	if err != nil {
		return Result{}, fmt.Errorf("execute command through HDU Station guest runtime: %w", err)
	}
	if response.Error != "" {
		return Result{}, errors.New(response.Error)
	}
	return Result{Stdout: response.Stdout, Stderr: response.Stderr, ExitCode: response.ExitCode}, nil
}

func (e *wslExecutor) refreshStatus(ctx context.Context) {
	if e.wsl == "" {
		e.statusMu.Lock()
		e.status.Ready = false
		e.statusMu.Unlock()
		return
	}
	exists, err := e.distributionExistsWithError(ctx)
	e.statusMu.Lock()
	defer e.statusMu.Unlock()
	if err != nil {
		e.status.Ready = false
		e.status.Reason = "cannot inspect Windows WSL2 distributions"
		return
	}
	if !exists {
		e.status.Ready = false
		e.status.Reason = "Windows WSL2 distribution is not installed"
		return
	}
	if !e.ownershipMarkerValid() {
		e.status.Ready = false
		e.status.Reason = "a same-named WSL2 distribution is not owned by HDU Station"
		return
	}
	e.status.Ready = true
	e.status.Reason = "Windows WSL2 private distribution is ready"
}

func (e *wslExecutor) ownershipPath() string {
	return filepath.Join(e.dataRoot, "sandbox", wslOwnershipFile)
}

func (e *wslExecutor) ownershipMarkerValid() bool {
	data, err := os.ReadFile(e.ownershipPath())
	if err != nil {
		return false
	}
	var marker wslOwnership
	if err := json.Unmarshal(data, &marker); err != nil {
		return false
	}
	return marker.Version == 1 && marker.Distribution == e.distribution
}

func (e *wslExecutor) ownedDistributionExistsWithError(ctx context.Context) (bool, error) {
	exists, err := e.distributionExistsWithError(ctx)
	if err != nil || !exists {
		return false, err
	}
	return e.ownershipMarkerValid(), nil
}

func (e *wslExecutor) writeOwnershipMarker() error {
	if strings.TrimSpace(e.dataRoot) == "" {
		return errors.New("sandbox data root is required")
	}
	data, err := json.Marshal(wslOwnership{Version: 1, Distribution: e.distribution})
	if err != nil {
		return fmt.Errorf("encode HDU Station WSL2 ownership marker: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(e.dataRoot, "sandbox"), 0o700); err != nil {
		return fmt.Errorf("create HDU Station WSL2 ownership directory: %w", err)
	}
	if err := writeWSLOwnershipMarker(e.ownershipPath(), data); err != nil {
		return fmt.Errorf("write HDU Station WSL2 ownership marker: %w", err)
	}
	return nil
}

func writeWSLOwnershipMarker(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".wsl-ownership-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func (e *wslExecutor) distributionExistsWithError(ctx context.Context) (bool, error) {
	output, err := e.run(ctx, e.wsl, "--list", "--quiet")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(output), "\x00", ""), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if line == e.distribution {
			return true, nil
		}
	}
	return false, nil
}

func (e *wslExecutor) loadImage(ctx context.Context) (sandboxImageManifest, string, error) {
	manifest, err := ensureSandboxImage(ctx, e.dataRoot, e.image, expectedSandboxPlatform(), "archive")
	if err != nil {
		return sandboxImageManifest{}, "", err
	}
	if manifest.Archive == "" {
		return sandboxImageManifest{}, "", errors.New("sandbox image manifest has no WSL archive")
	}
	archive, err := verifySandboxArtifact(e.dataRoot, manifest.Archive, manifest.Platform, manifest.SHA256)
	if err != nil {
		return sandboxImageManifest{}, "", err
	}
	return manifest, archive, nil
}

func (e *wslExecutor) hardenGuest(ctx context.Context) error {
	// Keep the command string constant and host-independent. The imported image
	// remains the only filesystem visible to this process; Windows drives and
	// Windows executable interop are disabled before the first user command.
	const script = "mkdir -p /etc && printf '%b' '[automount]\\nenabled=false\\n[interop]\\nenabled=false\\nappendWindowsPath=false\\n' > /etc/wsl.conf"
	if _, err := e.run(ctx, e.wsl, "--distribution", e.distribution, "--user", "root", "--exec", "/bin/sh", "-c", script); err != nil {
		return fmt.Errorf("harden HDU Station WSL2 distribution: %w", err)
	}
	return nil
}

func runWSLRuntime(ctx context.Context, binary, distribution string, request sandboxRuntimeRequest) (sandboxRuntimeResponse, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return sandboxRuntimeResponse{}, fmt.Errorf("encode WSL guest runtime request: %w", err)
	}
	commandContext, cancel := withDefaultTimeout(ctx, wslRuntimeTimeout)
	defer cancel()
	command := exec.CommandContext(commandContext, binary, "--distribution", distribution, "--user", "root", "--exec", wslRuntimePath, "--stdio")
	command.Stdin = bytes.NewReader(append(data, '\n'))
	output := &limitedBuffer{limit: maxSandboxRuntimeResponseBytes}
	command.Stdout = output
	command.Stderr = io.Discard
	err = command.Run()
	if err != nil {
		return sandboxRuntimeResponse{}, fmt.Errorf("run WSL guest runtime: %w", err)
	}
	if output.truncated {
		return sandboxRuntimeResponse{}, errors.New("WSL guest runtime response exceeds the 8 MiB limit")
	}
	response, err := decodeSandboxRuntimeResponse(bytes.NewReader(output.Bytes()))
	if err != nil {
		return sandboxRuntimeResponse{}, fmt.Errorf("decode WSL guest runtime response: %w", err)
	}
	if response.Version != sandboxRuntimeProtocolVersion {
		return sandboxRuntimeResponse{}, fmt.Errorf("WSL guest runtime protocol version %d is unsupported", response.Version)
	}
	return response, nil
}

func withDefaultTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

type limitedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	remaining := buffer.limit - buffer.Len()
	if remaining <= 0 {
		buffer.truncated = true
		return len(data), nil
	}
	if len(data) > remaining {
		_, _ = buffer.Buffer.Write(data[:remaining])
		buffer.truncated = true
		return len(data), nil
	}
	return buffer.Buffer.Write(data)
}

var _ Lifecycle = (*wslExecutor)(nil)
