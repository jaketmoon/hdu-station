package sandbox

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const sandboxRuntimeProtocolVersion = 1

const (
	maxSandboxRuntimeRequestBytes  = 1 << 20
	maxSandboxRuntimeResponseBytes = 8 << 20
)

type sandboxRuntimeRequest struct {
	Version int      `json:"version"`
	Op      string   `json:"op"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
}

type sandboxRuntimeResponse struct {
	Version  int    `json:"version"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

func callSandboxRuntime(ctx context.Context, socketPath string, request sandboxRuntimeRequest) (Result, error) {
	if strings.TrimSpace(socketPath) == "" {
		return Result{}, errors.New("sandbox runtime socket is required")
	}
	if request.Version == 0 {
		request.Version = sandboxRuntimeProtocolVersion
	}
	if request.Op == "" {
		return Result{}, errors.New("sandbox runtime operation is required")
	}
	var dialer net.Dialer
	connection, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return Result{}, fmt.Errorf("connect to sandbox runtime: %w", err)
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	} else {
		_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
	}
	requestData, err := encodeSandboxRuntimeRequest(request)
	if err != nil {
		return Result{}, err
	}
	if _, err := connection.Write(requestData); err != nil {
		return Result{}, fmt.Errorf("send sandbox runtime request: %w", err)
	}
	// The guest runtime serves one request per connection in this backend. Half
	// closing the write side tells its NDJSON scanner that no second request is
	// coming, which lets the host enforce the one-response contract without
	// guessing how long the guest may keep an idle socket open.
	if closer, ok := connection.(interface{ CloseWrite() error }); ok {
		if err := closer.CloseWrite(); err != nil {
			return Result{}, fmt.Errorf("close sandbox runtime request stream: %w", err)
		}
	} else {
		return Result{}, errors.New("sandbox runtime connection does not support half-close")
	}
	response, err := decodeSandboxRuntimeResponse(connection)
	if err != nil {
		return Result{}, fmt.Errorf("decode sandbox runtime response: %w", err)
	}
	if response.Version != sandboxRuntimeProtocolVersion {
		return Result{}, fmt.Errorf("sandbox runtime protocol version %d is unsupported", response.Version)
	}
	if response.Error != "" {
		return Result{}, errors.New(response.Error)
	}
	return Result{Stdout: response.Stdout, Stderr: response.Stderr, ExitCode: response.ExitCode}, nil
}

func encodeSandboxRuntimeRequest(request sandboxRuntimeRequest) ([]byte, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode sandbox runtime request: %w", err)
	}
	if len(data) > maxSandboxRuntimeRequestBytes {
		return nil, errors.New("sandbox runtime request exceeds the 1 MiB limit")
	}
	return append(data, '\n'), nil
}

// decodeSandboxRuntimeResponse reads exactly one bounded NDJSON response line.
// json.Decoder would accept a valid JSON object followed by arbitrary trailing
// bytes, which would make a malformed guest response look successful.
func decodeSandboxRuntimeResponse(reader io.Reader) (sandboxRuntimeResponse, error) {
	buffered := bufio.NewReaderSize(reader, 32<<10)
	line := make([]byte, 0, 4<<10)
	for {
		fragment, err := buffered.ReadSlice('\n')
		if len(line)+len(fragment) > maxSandboxRuntimeResponseBytes {
			return sandboxRuntimeResponse{}, errors.New("sandbox runtime response exceeds the 8 MiB limit")
		}
		line = append(line, fragment...)
		if err == nil {
			break
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			if len(line) == 0 {
				return sandboxRuntimeResponse{}, errors.New("sandbox runtime response is empty")
			}
			break
		}
		return sandboxRuntimeResponse{}, err
	}
	// A guest is allowed to keep its response line buffered until the host
	// closes the request side, but a second line is never part of this
	// single-call contract. This is the same strict JSON-EOF check used by
	// hdu-mate's runtime client.
	if _, trailingErr := buffered.ReadByte(); trailingErr == nil {
		return sandboxRuntimeResponse{}, errors.New("sandbox runtime response contains trailing data")
	} else if !errors.Is(trailingErr, io.EOF) {
		return sandboxRuntimeResponse{}, trailingErr
	}
	if len(line) == 0 {
		return sandboxRuntimeResponse{}, errors.New("sandbox runtime response is empty")
	}
	var response sandboxRuntimeResponse
	if err := json.Unmarshal(line, &response); err != nil {
		return sandboxRuntimeResponse{}, err
	}
	return response, nil
}
