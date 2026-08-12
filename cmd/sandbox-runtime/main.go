package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	runtimeProtocolVersion  = 1
	maxRequestBytes         = 1 << 20
	maxRuntimeOutput        = 4 << 20
	maxRuntimeCommandBytes  = 256
	maxRuntimeArguments     = 64
	maxRuntimeArgumentBytes = 4096
	commandTimeout          = 10 * time.Minute
)

type runtimeRequest struct {
	Version int      `json:"version"`
	Op      string   `json:"op"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
}

type runtimeResponse struct {
	Version  int    `json:"version"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

func main() {
	stdio := flag.Bool("stdio", false, "serve the runtime protocol over stdin/stdout")
	port := flag.Uint("vsock-port", 1024, "AF_VSOCK port to listen on")
	flag.Parse()

	if *stdio {
		if err := serveNDJSON(os.Stdin, os.Stdout); err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	listener, err := listenVsock(uint32(*port))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer listener.Close()
	for {
		connection, err := listener.Accept()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		go func() {
			defer connection.Close()
			_ = serveNDJSON(connection, connection)
		}()
	}
}

func serveNDJSON(reader io.Reader, writer io.Writer) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4<<10), maxRequestBytes)
	encoder := json.NewEncoder(writer)
	if !scanner.Scan() {
		return scanner.Err()
	}

	var request runtimeRequest
	if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
		return encoder.Encode(runtimeResponse{Version: runtimeProtocolVersion, Error: "invalid runtime request"})
	}
	return encoder.Encode(executeRequest(request))
}

func executeRequest(request runtimeRequest) runtimeResponse {
	if request.Version == 0 {
		request.Version = runtimeProtocolVersion
	}
	if request.Version != runtimeProtocolVersion {
		return runtimeResponse{Version: runtimeProtocolVersion, Error: "unsupported runtime protocol version"}
	}
	switch request.Op {
	case "ping":
		return runtimeResponse{Version: runtimeProtocolVersion}
	case "execute":
	default:
		return runtimeResponse{Version: runtimeProtocolVersion, Error: "unsupported runtime operation"}
	}
	if strings.TrimSpace(request.Command) == "" || strings.ContainsRune(request.Command, '\x00') || len([]byte(request.Command)) > maxRuntimeCommandBytes {
		return runtimeResponse{Version: runtimeProtocolVersion, Error: "runtime command is required, must not contain NUL, and must be at most 256 bytes"}
	}
	if len(request.Args) > maxRuntimeArguments {
		return runtimeResponse{Version: runtimeProtocolVersion, Error: "runtime supports at most 64 arguments"}
	}
	for _, argument := range request.Args {
		if strings.ContainsRune(argument, '\x00') || len([]byte(argument)) > maxRuntimeArgumentBytes {
			return runtimeResponse{Version: runtimeProtocolVersion, Error: "runtime arguments must not contain NUL and must be at most 4096 bytes each"}
		}
	}

	commandContext, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	command := exec.CommandContext(commandContext, request.Command, request.Args...)
	stdout := &limitedBuffer{limit: maxRuntimeOutput}
	stderr := &limitedBuffer{limit: maxRuntimeOutput}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	response := runtimeResponse{
		Version:  runtimeProtocolVersion,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}
	if stdout.truncated || stderr.truncated {
		response.Error = "runtime output exceeds the 4 MiB limit"
		return response
	}
	if errors.Is(commandContext.Err(), context.DeadlineExceeded) {
		response.Error = "runtime command exceeded the 10 minute limit"
		return response
	}
	if err == nil {
		return response
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		response.ExitCode = exitError.ExitCode()
		return response
	}
	response.Error = "runtime command failed to start"
	return response
}

type limitedBuffer struct {
	data      []byte
	limit     int
	truncated bool
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	remaining := buffer.limit - len(buffer.data)
	if remaining <= 0 {
		buffer.truncated = true
		return len(data), nil
	}
	if len(data) > remaining {
		buffer.data = append(buffer.data, data[:remaining]...)
		buffer.truncated = true
		return len(data), nil
	}
	buffer.data = append(buffer.data, data...)
	return len(data), nil
}

func (buffer *limitedBuffer) String() string { return string(buffer.data) }
