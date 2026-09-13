package campusauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const apiRoot = "https://api.hduhelp.com/hduhelp-neo"
const approvalURL = "https://neo.hduhelp.com/account/tokens/authorize"

type apiClient struct{ client *http.Client }

func newAPIClient() *apiClient {
	return &apiClient{client: &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type deviceError string

func (e deviceError) Error() string { return string(e) }

// Fixed routes, bounded responses, no redirects, no upstream error reflection.
// OAuth device endpoints return unwrapped RFC 8628 JSON, unlike campus queries.
func (a *apiClient) request(ctx context.Context, method, path string, form url.Values, bearer, deviceID string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, apiRoot+path, strings.NewReader(form.Encode()))
	if err != nil {
		return ErrResponse
	}
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if deviceID != "" {
		req.Header.Set("x-device-id", deviceID)
		req.Header.Set("x-device-name", "HDU Station")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return ErrUnavailable
	}
	if resp.StatusCode == 401 && method == http.MethodDelete {
		return nil
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return ErrResponse
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return ErrResponse
	}
	var failure struct {
		Error string `json:"error"`
		Code  *int   `json:"code"`
	}
	if json.Unmarshal(data, &failure) != nil && !(resp.StatusCode == http.StatusNoContent && len(data) == 0 && out == nil) {
		return ErrResponse
	}
	switch failure.Error {
	case "authorization_pending", "slow_down", "expired_token", "access_denied", "invalid_grant":
		return deviceError(failure.Error)
	}
	if failure.Error != "" || (failure.Code != nil && *failure.Code != 0) || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrResponse
	}
	if out != nil && json.Unmarshal(data, out) != nil {
		return ErrResponse
	}
	return nil
}

type deviceResponse struct {
	DeviceCode string `json:"device_code"`
	UserCode   string `json:"user_code"`
	URL        string `json:"verification_uri_complete"`
	ExpiresIn  int64  `json:"expires_in"`
	Interval   int64  `json:"interval"`
}

func (a *apiClient) begin(ctx context.Context, deviceID string) (deviceResponse, error) {
	var data deviceResponse
	// Users can adjust this requested expiry on the official approval page.
	values := url.Values{"client_id": {"hduhelp-cli"}, "scope": {requestedScope}, "device_name": {"HDU Station"}, "device_id": {deviceID}, "expires_at": {formatMillis(time.Now().Add(30 * 24 * time.Hour))}}
	err := a.request(ctx, http.MethodPost, "/open-apis/auth/device-authorization", values, "", "", &data)
	if err != nil {
		return deviceResponse{}, err
	}
	if data.DeviceCode == "" || !validToken(data.DeviceCode) || data.UserCode == "" || len(data.UserCode) > 32 || !validToken(data.UserCode) || data.ExpiresIn < 1 || data.ExpiresIn > 900 || data.Interval < 1 || data.Interval > data.ExpiresIn {
		return deviceResponse{}, ErrResponse
	}
	u, err := url.Parse(data.URL)
	if err != nil || u.Scheme != "https" || u.Host != "neo.hduhelp.com" || u.Path != "/account/tokens/authorize" || u.User != nil || u.Fragment != "" || u.RawPath != "" {
		return deviceResponse{}, ErrResponse
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(query) != 1 || len(query["user_code"]) != 1 || query.Get("user_code") != data.UserCode {
		return deviceResponse{}, ErrResponse
	}
	data.URL = approvalURL + "?" + query.Encode()
	if data.Interval < 5 {
		data.Interval = 5
	}
	return data, nil
}

type tokenResponse struct {
	Token     string `json:"access_token"`
	Scope     string `json:"scope"`
	ExpiresIn int64  `json:"expires_in"`
	TokenType string `json:"token_type"`
}

func (a *apiClient) poll(ctx context.Context, deviceCode, deviceID string) (tokenResponse, error) {
	var data tokenResponse
	err := a.request(ctx, http.MethodPost, "/open-apis/authen/access-token", url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "client_id": {"hduhelp-cli"}, "device_code": {deviceCode}}, "", deviceID, &data)
	if err != nil {
		return tokenResponse{}, err
	}
	if data.Token == "" || !validToken(data.Token) || !strings.EqualFold(data.TokenType, "Bearer") || data.ExpiresIn < 0 || data.ExpiresIn > 100*365*24*3600 || len(data.Scope) > 4096 {
		return tokenResponse{}, ErrResponse
	}
	return data, nil
}

func (a *apiClient) cancel(ctx context.Context, deviceCode, deviceID string) error {
	err := a.request(ctx, http.MethodPost, "/open-apis/auth/device-authorization/cancel", url.Values{"client_id": {"hduhelp-cli"}, "device_code": {deviceCode}}, "", deviceID, nil)
	if err == deviceError("expired_token") || err == deviceError("invalid_grant") {
		return nil
	}
	return err
}

func (a *apiClient) revoke(ctx context.Context, token string) error {
	return a.request(ctx, http.MethodDelete, "/cli/tokens/current", nil, token, "", nil)
}
