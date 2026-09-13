package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var sourceDiagnosticMu sync.Mutex

// Only fixed operation names, durations and error categories reach this file.
// Queries, URLs, response bodies and credentials are deliberately absent.
func recordXiaohongshuRequest(root, path string, attempt int, start time.Time, err error) {
	if root == "" {
		return
	}
	operation := map[string]string{"/api/v1/login/status": "status", "/api/v1/feeds/search": "search", "/api/v1/feeds/detail": "read"}[path]
	if operation == "" {
		return
	}
	outcome := "ok"
	if err != nil {
		outcome = "unavailable"
		for _, kind := range []struct {
			err  error
			name string
		}{
			{context.Canceled, "canceled"}, {context.DeadlineExceeded, "deadline"},
			{errSourceTimeout, "timeout"}, {errSourceAuth, "auth_failed"},
			{errSourceRateLimit, "rate_limited"}, {errSourceServer, "server_error"},
			{errSourceResponse, "invalid_response"},
			{errSourceVerification, "verification_required"},
		} {
			if errors.Is(err, kind.err) {
				outcome = kind.name
				break
			}
		}
	}
	entry, _ := json.Marshal(struct {
		At        string `json:"at"`
		Operation string `json:"operation"`
		Attempt   int    `json:"attempt"`
		ElapsedMS int64  `json:"elapsedMs"`
		Outcome   string `json:"outcome"`
	}{start.Format(time.RFC3339), operation, attempt, time.Since(start).Milliseconds(), outcome})
	sourceDiagnosticMu.Lock()
	defer sourceDiagnosticMu.Unlock()
	dir := filepath.Join(root, "logs")
	if os.MkdirAll(dir, 0700) != nil {
		return
	}
	file := filepath.Join(dir, "xiaohongshu-requests.jsonl")
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if info, err := os.Stat(file); err == nil && info.Size() >= 1<<20 {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	f, err := os.OpenFile(file, flags, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(entry, '\n'))
}
