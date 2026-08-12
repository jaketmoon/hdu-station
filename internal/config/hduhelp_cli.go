package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
)

const hduhelpAPIHost = "api.hduhelp.com"

// LoadHduhelpCLIPAT reads a hduhelp-cli credential only after an explicit
// Station import request. It returns the PAT for immediate saving to Station's
// private configuration, never a status DTO or loggable diagnostic.
func LoadHduhelpCLIPAT(path string, now time.Time) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read hduhelp-cli credential: %w", err)
	}
	var credential struct {
		Server    string `json:"server"`
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&credential); err != nil {
		return "", fmt.Errorf("parse hduhelp-cli credential: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", errors.New("parse hduhelp-cli credential: multiple JSON values are not allowed")
		}
		return "", fmt.Errorf("parse hduhelp-cli credential: %w", err)
	}
	server, err := url.Parse(strings.TrimSpace(credential.Server))
	if err != nil || server.Scheme != "https" || server.Host != hduhelpAPIHost || server.User != nil || server.RawQuery != "" || server.Fragment != "" || (server.Path != "" && server.Path != "/") {
		return "", errors.New("hduhelp-cli credential is not for the official HDUHelp API")
	}
	token := strings.TrimSpace(credential.Token)
	if !strings.HasPrefix(token, "hduhelp_pat_") || len(token) < 32 {
		return "", errors.New("hduhelp-cli credential has an invalid personal access token")
	}
	if expiresAt := strings.TrimSpace(credential.ExpiresAt); expiresAt != "" {
		expires, parseErr := time.Parse(time.RFC3339, expiresAt)
		if parseErr != nil {
			return "", errors.New("hduhelp-cli credential has an invalid expiry")
		}
		if !expires.After(now) {
			return "", errors.New("hduhelp-cli credential has expired")
		}
	}
	return token, nil
}
