package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestServeNDJSONHandlesOneRequestAndReturns(t *testing.T) {
	input := strings.NewReader("{\"version\":1,\"op\":\"ping\"}\n{\"version\":1,\"op\":\"execute\",\"command\":\"/bin/echo\",\"args\":[\"guest\"]}\n")
	var output bytes.Buffer
	if err := serveNDJSON(input, &output); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	var ping runtimeResponse
	if err := decoder.Decode(&ping); err != nil {
		t.Fatal(err)
	}
	if ping.Version != runtimeProtocolVersion || ping.Error != "" {
		t.Fatalf("unexpected ping response: %#v", ping)
	}
	var extra runtimeResponse
	if err := decoder.Decode(&extra); err == nil {
		t.Fatalf("runtime must return exactly one response, got %#v", extra)
	}
}

func TestServeNDJSONReturnsInvalidRequestResponse(t *testing.T) {
	var output bytes.Buffer
	if err := serveNDJSON(strings.NewReader("not-json\n"), &output); err != nil {
		t.Fatal(err)
	}
	var response runtimeResponse
	if err := json.NewDecoder(&output).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Error != "invalid runtime request" {
		t.Fatalf("unexpected invalid request response: %#v", response)
	}
}

func TestExecuteRequestRejectsNulAndUnsupportedOperations(t *testing.T) {
	for _, request := range []runtimeRequest{
		{Version: runtimeProtocolVersion, Op: "execute", Command: "echo\x00bad"},
		{Version: runtimeProtocolVersion, Op: "shell", Command: "echo"},
	} {
		response := executeRequest(request)
		if response.Error == "" {
			t.Fatalf("request was accepted: %#v", request)
		}
	}
}

func TestExecuteRequestRejectsOversizedCommandAndArguments(t *testing.T) {
	tooManyArguments := make([]string, maxRuntimeArguments+1)
	for index := range tooManyArguments {
		tooManyArguments[index] = "arg"
	}
	for _, request := range []runtimeRequest{
		{Version: runtimeProtocolVersion, Op: "execute", Command: strings.Repeat("c", maxRuntimeCommandBytes+1)},
		{Version: runtimeProtocolVersion, Op: "execute", Command: "echo", Args: tooManyArguments},
		{Version: runtimeProtocolVersion, Op: "execute", Command: "echo", Args: []string{strings.Repeat("a", maxRuntimeArgumentBytes+1)}},
	} {
		response := executeRequest(request)
		if response.Error == "" {
			t.Fatalf("oversized runtime request was accepted: command=%d args=%d", len(request.Command), len(request.Args))
		}
	}
}

func TestExecuteRequestPreservesGuestExitCode(t *testing.T) {
	response := executeRequest(runtimeRequest{
		Version: runtimeProtocolVersion,
		Op:      "execute",
		Command: "/bin/sh",
		Args:    []string{"-c", "exit 7"},
	})
	if response.Error != "" || response.ExitCode != 7 {
		t.Fatalf("unexpected guest exit response: %#v", response)
	}
}
