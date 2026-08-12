package sandbox

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"
)

func TestSandboxRuntimeProtocolUsesUnixSocketAndReturnsGuestResult(t *testing.T) {
	temporary, err := os.CreateTemp("/tmp", "hdu-station-runtime-*.sock")
	if err != nil {
		t.Fatal(err)
	}
	socketPath := temporary.Name()
	_ = temporary.Close()
	_ = os.Remove(socketPath)
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer connection.Close()
		var request sandboxRuntimeRequest
		if err := json.NewDecoder(connection).Decode(&request); err != nil {
			done <- err
			return
		}
		if request.Version != sandboxRuntimeProtocolVersion || request.Op != "execute" || request.Command != "python" {
			done <- &protocolTestError{"unexpected runtime request"}
			return
		}
		if err := json.NewEncoder(connection).Encode(sandboxRuntimeResponse{Version: sandboxRuntimeProtocolVersion, Stdout: "guest-only", ExitCode: 0}); err != nil {
			done <- err
			return
		}
		// The real guest closes the one-request connection after its scanner
		// reaches EOF. Mirror that here so the host can enforce strict EOF.
		_ = connection.Close()
		done <- nil
	}()

	result, err := callSandboxRuntime(context.Background(), socketPath, sandboxRuntimeRequest{Op: "execute", Command: "python", Args: []string{"-c", "print(1)"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "guest-only" || result.ExitCode != 0 {
		t.Fatalf("unexpected guest result: %#v", result)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestDecodeSandboxRuntimeResponseRejectsTrailingData(t *testing.T) {
	_, err := decodeSandboxRuntimeResponse(strings.NewReader(`{"version":1} trailing`))
	if err == nil {
		t.Fatal("runtime response with trailing data was accepted")
	}
}

func TestDecodeSandboxRuntimeResponseRejectsSecondLine(t *testing.T) {
	_, err := decodeSandboxRuntimeResponse(strings.NewReader("{\"version\":1}\n{\"version\":1}\n"))
	if err == nil || !strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("runtime response with a second line error = %v", err)
	}
}

func TestDecodeSandboxRuntimeResponseEnforcesBound(t *testing.T) {
	_, err := decodeSandboxRuntimeResponse(strings.NewReader(strings.Repeat("x", maxSandboxRuntimeResponseBytes+1)))
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized runtime response error = %v", err)
	}
}

type protocolTestError struct{ message string }

func (err *protocolTestError) Error() string { return err.message }
