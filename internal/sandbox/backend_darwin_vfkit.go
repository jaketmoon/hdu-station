//go:build darwin

package sandbox

import (
	"context"
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
	vfkitExecutable    = "vfkit"
	vfkitVsockPort     = 1024
	vfkitMemoryMiB     = "2048"
	vfkitCPUs          = "2"
	vfkitRuntimeSocket = "runtime.sock"
)

type vfkitExecutor struct {
	mu            sync.Mutex
	dataRoot      string
	image         ImageSource
	vfkit         string
	vfkitSource   string
	runtimeSocket string
	status        Status
	process       *exec.Cmd
	done          chan error
	runtimeReady  bool
}

func newPlatformExecutor(dataRoot, goos, goarch string, image ImageSource, status Status) Executor {
	if goos != "darwin" || goarch != "arm64" {
		return &unavailableExecutor{status: status}
	}
	vfkit, source := resolveVFKitPath(dataRoot)
	executor := &vfkitExecutor{
		dataRoot:      dataRoot,
		image:         image,
		vfkit:         vfkit,
		vfkitSource:   source,
		runtimeSocket: filepath.Join(dataRoot, "sandbox", vfkitRuntimeSocket),
		status:        status,
	}
	executor.refreshStatus()
	return executor
}

func (e *vfkitExecutor) Status(context.Context) Status {
	e.refreshStatus()
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status
}

func (e *vfkitExecutor) Install(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if e.dataRoot == "" {
		return errors.New("sandbox data root is required")
	}
	if err := e.ensureVFKit(ctx); err != nil {
		return err
	}
	if _, err := e.ensureImage(ctx); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(e.dataRoot, "sandbox"), 0o700); err != nil {
		return fmt.Errorf("create Station sandbox directory: %w", err)
	}
	e.refreshStatus()
	return nil
}

func (e *vfkitExecutor) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if e.dataRoot == "" {
		return errors.New("sandbox data root is required")
	}
	e.mu.Lock()
	processRunning, runtimeReady := e.process != nil, e.runtimeReady
	e.mu.Unlock()
	if processRunning {
		if runtimeReady {
			return nil
		}
		return e.waitForReady(ctx)
	}
	if err := e.ensureVFKit(ctx); err != nil {
		return err
	}
	disk, err := e.ensureImage(ctx)
	if err != nil {
		return err
	}
	if len(e.runtimeSocket) >= 100 {
		return errors.New("sandbox runtime socket path is too long")
	}
	if err := os.MkdirAll(filepath.Dir(e.runtimeSocket), 0o700); err != nil {
		return fmt.Errorf("create Sandbox runtime directory: %w", err)
	}
	e.mu.Lock()
	if e.process != nil {
		runtimeReady := e.runtimeReady
		e.mu.Unlock()
		if runtimeReady {
			return nil
		}
		return e.waitForReady(ctx)
	}
	e.runtimeReady = false
	if err := os.Remove(e.runtimeSocket); err != nil && !errors.Is(err, os.ErrNotExist) {
		e.mu.Unlock()
		return fmt.Errorf("remove stale Sandbox runtime socket: %w", err)
	}
	efi := filepath.Join(e.dataRoot, "sandbox", "efi-variables.fd")
	command := exec.Command(e.vfkit,
		"--log-level", "error",
		"--cpus", vfkitCPUs,
		"--memory", vfkitMemoryMiB,
		"--bootloader", "efi,variable-store="+efi+",create",
		"--device", "virtio-blk,path="+disk,
		"--device", "virtio-net,nat",
		"--device", fmt.Sprintf("virtio-vsock,port=%d,socketURL=%s,connect", vfkitVsockPort, e.runtimeSocket),
		"--device", "virtio-rng",
	)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		e.mu.Unlock()
		return fmt.Errorf("start vfkit Sandbox: %w", err)
	}
	done := make(chan error, 1)
	e.process = command
	e.done = done
	e.mu.Unlock()
	go e.wait(command, done)

	if err := e.waitForReady(ctx); err != nil {
		stopContext, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = e.Stop(stopContext)
		stopCancel()
		return err
	}
	e.refreshStatus()
	return nil
}

func (e *vfkitExecutor) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}
	e.mu.Lock()
	process, done := e.process, e.done
	e.runtimeReady = false
	e.mu.Unlock()
	if process == nil || done == nil {
		return nil
	}
	if process.Process != nil {
		_ = process.Process.Signal(os.Interrupt)
	}
	select {
	case <-done:
		e.refreshStatus()
		return nil
	case <-ctx.Done():
		if process.Process != nil {
			_ = process.Process.Kill()
		}
		<-done
		e.refreshStatus()
		return ctx.Err()
	}
}

func (e *vfkitExecutor) waitForReady(ctx context.Context) error {
	readyContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := e.waitForRuntime(readyContext); err != nil {
		return err
	}
	e.refreshStatus()
	return nil
}

func (e *vfkitExecutor) Purge(ctx context.Context) error {
	if err := e.Stop(ctx); err != nil {
		return err
	}
	cleanedDataRoot := filepath.Clean(strings.TrimSpace(e.dataRoot))
	if cleanedDataRoot == "" || cleanedDataRoot == "." || cleanedDataRoot == string(filepath.Separator) || filepath.Dir(cleanedDataRoot) == cleanedDataRoot {
		return errors.New("sandbox data root is required")
	}
	sandboxRoot := filepath.Join(cleanedDataRoot, "sandbox")
	if filepath.Clean(sandboxRoot) == filepath.Clean(filepath.Dir(cleanedDataRoot)) || filepath.Clean(sandboxRoot) == "." {
		return errors.New("refusing to purge an unsafe sandbox path")
	}
	if err := os.RemoveAll(sandboxRoot); err != nil {
		return fmt.Errorf("purge Station Sandbox data: %w", err)
	}
	e.mu.Lock()
	e.vfkit = ""
	e.vfkitSource = ""
	e.mu.Unlock()
	e.refreshStatus()
	return nil
}

func (e *vfkitExecutor) Execute(ctx context.Context, request Request) (Result, error) {
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	e.mu.Lock()
	running := e.process != nil
	runtimeReady := e.runtimeReady
	e.mu.Unlock()
	if !running || !runtimeReady {
		return Result{}, fmt.Errorf("%w: macOS vfkit Sandbox is not started", ErrUnavailable)
	}
	return callSandboxRuntime(ctx, e.runtimeSocket, sandboxRuntimeRequest{
		Version: sandboxRuntimeProtocolVersion,
		Op:      "execute",
		Command: request.Command,
		Args:    request.Args,
	})
}

func (e *vfkitExecutor) wait(command *exec.Cmd, done chan<- error) {
	err := command.Wait()
	e.mu.Lock()
	if e.process == command {
		e.process = nil
		e.done = nil
		e.runtimeReady = false
		e.status.Ready = false
		e.status.Reason = "macOS vfkit Sandbox stopped"
	}
	e.mu.Unlock()
	done <- err
}

func (e *vfkitExecutor) waitForRuntime(ctx context.Context) error {
	for {
		e.mu.Lock()
		processRunning := e.process != nil
		e.mu.Unlock()
		if !processRunning {
			return errors.New("macOS Sandbox process exited before guest runtime became ready")
		}
		if _, err := os.Stat(e.runtimeSocket); err == nil {
			pingContext, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			_, callErr := callSandboxRuntime(pingContext, e.runtimeSocket, sandboxRuntimeRequest{
				Version: sandboxRuntimeProtocolVersion,
				Op:      "ping",
			})
			cancel()
			if callErr == nil {
				e.mu.Lock()
				if e.process != nil {
					e.runtimeReady = true
				}
				e.mu.Unlock()
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for macOS Sandbox runtime: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (e *vfkitExecutor) refreshStatus() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.process == nil || !e.runtimeReady {
		e.status.Ready = false
		if e.process != nil && !e.runtimeReady {
			e.status.Reason = "macOS Sandbox guest runtime is not ready"
			return
		}
		if e.vfkit == "" {
			e.status.Reason = "macOS vfkit sidecar is not installed"
			return
		}
		if _, err := os.Stat(filepath.Join(e.dataRoot, "sandbox", "image.json")); err != nil {
			e.status.Reason = "macOS Sandbox image is not installed"
		} else {
			e.status.Reason = "macOS vfkit Sandbox is not started"
		}
		return
	}
	e.status.Ready = true
	e.status.Reason = "macOS vfkit Sandbox is ready"
}

func (e *vfkitExecutor) ensureVFKit(ctx context.Context) error {
	if e.vfkit != "" {
		valid := false
		if e.vfkitSource == "station" {
			valid, _ = validVFKitInstall(e.vfkit, filepath.Join(filepath.Dir(e.vfkit), "install.json"), pinnedVFKitPackage)
		} else {
			_, err := os.Stat(e.vfkit)
			valid = err == nil
		}
		if valid {
			return nil
		}
		e.vfkit = ""
	}
	if strings.TrimSpace(e.dataRoot) == "" {
		return fmt.Errorf("%w: macOS vfkit sidecar is not installed", ErrUnavailable)
	}
	path, err := ensureVFKit(ctx, e.dataRoot, vfkitHTTPClient())
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.vfkit = path
	e.vfkitSource = "station"
	e.mu.Unlock()
	return nil
}

func (e *vfkitExecutor) ensureImage(ctx context.Context) (string, error) {
	manifest, err := ensureSandboxImage(ctx, e.dataRoot, e.image, expectedSandboxPlatform(), "disk")
	if err != nil {
		return "", err
	}
	if manifest.Disk == "" {
		return "", errors.New("sandbox image manifest has no macOS disk image")
	}
	return verifySandboxArtifact(e.dataRoot, manifest.Disk, manifest.Platform, manifest.SHA256)
}

var _ Lifecycle = (*vfkitExecutor)(nil)
