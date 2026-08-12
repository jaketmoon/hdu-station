package sandbox

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDetectSupportedPlatformsAreNotReportedReadyBeforeBackendInstall(t *testing.T) {
	for _, test := range []struct {
		goos   string
		goarch string
	}{
		{goos: "darwin", goarch: "arm64"},
		{goos: "windows", goarch: "amd64"},
	} {
		status := Detect(test.goos, test.goarch)
		if !status.Supported || status.Ready || status.Reason == "" {
			t.Fatalf("unexpected status for %s/%s: %#v", test.goos, test.goarch, status)
		}
	}
}

func TestUnsupportedPlatformFailsInsteadOfExecutingOnHost(t *testing.T) {
	executor := New("linux", "amd64")
	status := executor.Status(context.Background())
	if status.Supported || status.Ready {
		t.Fatalf("unexpected status: %#v", status)
	}
	_, err := executor.Execute(context.Background(), Request{Command: "echo", Args: []string{"unsafe"}})
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unexpected execution error: %v", err)
	}
}

func TestUnavailableExecutorRejectsEmptyCommand(t *testing.T) {
	_, err := New("darwin", "arm64").Execute(context.Background(), Request{})
	if err == nil || !strings.Contains(err.Error(), "command is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
