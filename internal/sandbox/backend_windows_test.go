//go:build windows

package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWSLInstallVerifiesImageAndHardensPrivateDistribution(t *testing.T) {
	root := t.TempDir()
	imageDir := filepath.Join(root, "sandbox", "images")
	if err := os.MkdirAll(imageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	archive := []byte("fake signed image payload")
	archivePath := filepath.Join(imageDir, "runtime.tar")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(archive)
	manifest, err := json.Marshal(sandboxImageManifest{Platform: "windows/amd64", Archive: "images/runtime.tar", SHA256: hex.EncodeToString(digest[:])})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sandbox", "image.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	installed := false
	runner := func(_ context.Context, _ string, arguments ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), arguments...))
		if len(arguments) == 2 && arguments[0] == "--list" && arguments[1] == "--quiet" {
			if installed {
				return []byte("HDU-Station-Sandbox\r\n"), nil
			}
			return []byte{}, nil
		}
		if len(arguments) > 0 && arguments[0] == "--import" {
			installed = true
		}
		return nil, nil
	}
	executor := &wslExecutor{dataRoot: root, distribution: stationDistribution, wsl: "wsl.exe", run: runner, status: Detect("windows", "amd64")}
	if err := executor.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !executor.Status(context.Background()).Ready {
		t.Fatal("WSL distribution should be ready after install")
	}
	if _, err := os.Stat(filepath.Join(root, "sandbox", wslOwnershipFile)); err != nil {
		t.Fatalf("WSL ownership marker was not written: %v", err)
	}
	joined := ""
	for _, call := range calls {
		joined += strings.Join(call, " ") + "\n"
	}
	if !strings.Contains(joined, "--import HDU-Station-Sandbox") || !strings.Contains(joined, "appendWindowsPath=false") {
		t.Fatalf("install did not import and harden the guest: %s", joined)
	}
}

func TestWSLDoesNotUseSameNamedUnownedDistribution(t *testing.T) {
	root := t.TempDir()
	calledRuntime := false
	unregistered := false
	executor := &wslExecutor{
		dataRoot:     root,
		distribution: stationDistribution,
		wsl:          "wsl.exe",
		run: func(_ context.Context, _ string, arguments ...string) ([]byte, error) {
			if len(arguments) == 2 && arguments[0] == "--list" && arguments[1] == "--quiet" {
				return []byte(stationDistribution + "\r\n"), nil
			}
			if len(arguments) > 0 && arguments[0] == "--unregister" {
				unregistered = true
			}
			return nil, nil
		},
		runtime: func(context.Context, string, string, sandboxRuntimeRequest) (sandboxRuntimeResponse, error) {
			calledRuntime = true
			return sandboxRuntimeResponse{}, nil
		},
		status: Detect("windows", "amd64"),
	}
	status := executor.Status(context.Background())
	if status.Ready || !strings.Contains(status.Reason, "not owned") {
		t.Fatalf("unowned WSL distribution was reported ready: %#v", status)
	}
	if _, err := executor.Execute(context.Background(), Request{Command: "whoami"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unowned WSL distribution was used: %v", err)
	}
	if err := executor.Purge(context.Background()); err != nil {
		t.Fatalf("purging unowned distribution should be a no-op: %v", err)
	}
	if calledRuntime || unregistered {
		t.Fatal("unowned WSL distribution was touched")
	}
}

func TestWSLPurgeRemovesStaleMarkerWhenDistributionIsGone(t *testing.T) {
	root := t.TempDir()
	executor := &wslExecutor{
		dataRoot:     root,
		distribution: stationDistribution,
		wsl:          "wsl.exe",
		run: func(_ context.Context, _ string, arguments ...string) ([]byte, error) {
			if len(arguments) == 2 && arguments[0] == "--list" && arguments[1] == "--quiet" {
				return nil, nil
			}
			return nil, nil
		},
		status: Detect("windows", "amd64"),
	}
	if err := executor.writeOwnershipMarker(); err != nil {
		t.Fatal(err)
	}
	if err := executor.Purge(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(executor.ownershipPath()); !os.IsNotExist(err) {
		t.Fatalf("stale ownership marker remains: %v", err)
	}
}

func TestWSLStateQueryFailureFailsClosed(t *testing.T) {
	root := t.TempDir()
	calledImport := false
	executor := &wslExecutor{
		dataRoot:     root,
		distribution: stationDistribution,
		wsl:          "wsl.exe",
		run: func(_ context.Context, _ string, arguments ...string) ([]byte, error) {
			if len(arguments) == 2 && arguments[0] == "--list" && arguments[1] == "--quiet" {
				return nil, errors.New("wsl unavailable")
			}
			calledImport = len(arguments) > 0 && arguments[0] == "--import"
			return nil, nil
		},
		status: Detect("windows", "amd64"),
	}
	if err := executor.Install(context.Background()); err == nil || !strings.Contains(err.Error(), "inspect WSL2 distributions") {
		t.Fatalf("Install error = %v", err)
	}
	if calledImport {
		t.Fatal("WSL import started after distribution query failed")
	}
}

func TestWSLDefaultTimeoutPreservesExistingDeadline(t *testing.T) {
	deadlineContext, deadlineCancel := context.WithTimeout(context.Background(), time.Second)
	defer deadlineCancel()
	withDeadline, cancel := withDefaultTimeout(deadlineContext, time.Minute)
	defer cancel()
	deadline, ok := withDeadline.Deadline()
	if !ok || !deadline.Equal(func() time.Time {
		value, _ := deadlineContext.Deadline()
		return value
	}()) {
		t.Fatal("default timeout replaced an existing deadline")
	}

	withoutDeadline, withoutCancel := withDefaultTimeout(context.Background(), time.Millisecond)
	defer withoutCancel()
	if _, ok := withoutDeadline.Deadline(); !ok {
		t.Fatal("default timeout did not add a deadline")
	}
}

func TestWSLOwnershipMarkerIsWrittenAtomically(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sandbox", wslOwnershipFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeWSLOwnershipMarker(path, []byte(`{"version":1,"distribution":"HDU-Station-Sandbox"}`)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"version":1,"distribution":"HDU-Station-Sandbox"}` {
		t.Fatalf("marker = %q", data)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != wslOwnershipFile {
		t.Fatalf("temporary marker files remain: %v", entries)
	}
}

func TestWSLExecuteUsesGuestRuntimeInsteadOfPassingUserCommandToWSL(t *testing.T) {
	called := false
	executor := &wslExecutor{
		dataRoot:     t.TempDir(),
		distribution: stationDistribution,
		wsl:          "wsl.exe",
		run: func(_ context.Context, _ string, arguments ...string) ([]byte, error) {
			if len(arguments) == 2 && arguments[0] == "--list" && arguments[1] == "--quiet" {
				return []byte(stationDistribution + "\r\n"), nil
			}
			for _, argument := range arguments {
				if argument == "rm" || argument == "-rf" || argument == "/tmp/user-data" {
					t.Fatalf("user command was passed to WSL control runner: %#v", arguments)
				}
			}
			return nil, nil
		},
		runtime: func(_ context.Context, binary, distribution string, request sandboxRuntimeRequest) (sandboxRuntimeResponse, error) {
			called = true
			if binary != "wsl.exe" || distribution != stationDistribution || request.Command != "rm" || len(request.Args) != 2 {
				t.Fatalf("unexpected guest runtime invocation: binary=%q distribution=%q request=%#v", binary, distribution, request)
			}
			return sandboxRuntimeResponse{Version: sandboxRuntimeProtocolVersion, Stdout: "guest-only", ExitCode: 0}, nil
		},
		status: Detect("windows", "amd64"),
	}
	if err := executor.writeOwnershipMarker(); err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(context.Background(), Request{Command: "rm", Args: []string{"-rf", "/tmp/user-data"}})
	if err != nil {
		t.Fatal(err)
	}
	if !called || result.Stdout != "guest-only" {
		t.Fatalf("guest runtime was not used: called=%v result=%#v", called, result)
	}
}

func TestWSLStartChecksGuestRuntimeBeforeReportingReady(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sandbox"), 0o700); err != nil {
		t.Fatal(err)
	}
	calledPing := false
	executor := &wslExecutor{
		dataRoot:     root,
		distribution: stationDistribution,
		wsl:          "wsl.exe",
		run: func(_ context.Context, _ string, arguments ...string) ([]byte, error) {
			if len(arguments) == 2 && arguments[0] == "--list" && arguments[1] == "--quiet" {
				return []byte(" * " + stationDistribution + "\r\n"), nil
			}
			return nil, nil
		},
		runtime: func(_ context.Context, _, _ string, request sandboxRuntimeRequest) (sandboxRuntimeResponse, error) {
			calledPing = request.Op == "ping"
			return sandboxRuntimeResponse{Version: sandboxRuntimeProtocolVersion, Error: "guest runtime missing"}, nil
		},
		status: Detect("windows", "amd64"),
	}
	if err := executor.writeOwnershipMarker(); err != nil {
		t.Fatal(err)
	}
	if err := executor.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "guest runtime missing") {
		t.Fatalf("Start error = %v", err)
	}
	if !calledPing {
		t.Fatal("Start did not check the guest runtime")
	}
}
