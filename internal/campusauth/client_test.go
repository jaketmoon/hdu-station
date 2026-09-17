package campusauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

const deviceJSON = `{"device_code":"device-private-sentinel","user_code":"ABCD-EFGH","verification_uri_complete":"https://neo.hduhelp.com/account/tokens/authorize?user_code=ABCD-EFGH","expires_in":600,"interval":5}`
const tokenJSON = `{"access_token":"new-private-sentinel","token_type":"Bearer","scope":"academic:course:read academic:schedule:read academic:coursesimulation:read academic:coursesimulation:write academic:coursefavorite:read academic:coursefavorite:write","expires_in":3600}`

func fixture(t *testing.T, handler roundTrip) (*Client, chan time.Duration, chan struct{}, string) {
	t.Helper()
	root := t.TempDir()
	c, err := New(root, "legacy-private-sentinel")
	if err != nil {
		t.Fatal(err)
	}
	c.http.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "api.hduhelp.com" || r.URL.RawQuery != "" {
			t.Error("credential request escaped fixed HTTPS routes")
		}
		return handler(r)
	})
	waits, ticks := make(chan time.Duration, 16), make(chan struct{}, 16)
	c.wait = func(ctx context.Context, d time.Duration) bool {
		waits <- d
		select {
		case <-ctx.Done():
			return false
		case <-ticks:
			return ctx.Err() == nil
		}
	}
	t.Cleanup(c.Close)
	return c, waits, ticks, root
}

func TestRevocationDoesNotReportSuccessForUpstreamErrors(t *testing.T) {
	for _, body := range []string{`{"code":500,"msg":"private-sentinel"}`, `{"error":"private-sentinel"}`, "private-sentinel", strings.Repeat("x", (64<<10)+1)} {
		c, _, _, _ := fixture(t, func(*http.Request) (*http.Response, error) { return response(200, body), nil })
		if err := c.http.revoke(context.Background(), "private-sentinel"); err == nil || strings.Contains(err.Error(), "private-sentinel") {
			t.Fatal("revocation failure hidden or reflected")
		}
	}
}

func TestLoginStatusUsesLocalCredentialsWithoutBusinessRequests(t *testing.T) {
	c, _, _, root := fixture(t, func(*http.Request) (*http.Response, error) {
		t.Error("local login status made a network request")
		return nil, ErrUnavailable
	})
	for _, tc := range []struct {
		name    string
		token   string
		expires int64
		want    string
	}{
		{"logged out", "", 0, "logged_out"},
		{"legacy token", "legacy-private-sentinel", 0, "saved"},
		{"web login", "private-sentinel", time.Now().Add(time.Hour).UnixMilli(), "saved"},
		{"expired login", "private-sentinel", time.Now().Add(-time.Hour).UnixMilli(), "expired"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c.credentials.Token, c.credentials.ExpiresAt = tc.token, tc.expires
			if err := c.save(c.credentials); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(root, "campus-auth.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if c.Status() != tc.want {
				t.Fatal("login status did not reflect local credentials or expiry")
			}
			after, err := os.ReadFile(filepath.Join(root, "campus-auth.yaml"))
			if err != nil || string(before) != string(after) {
				t.Fatal("reading status modified the saved login")
			}
		})
	}
	c.Close()
	if c.Status() != "logged_out" || (*Client)(nil).Status() != "logged_out" {
		t.Fatal("closed or uninitialized login reported as saved")
	}
}

func advance(t *testing.T, waits chan time.Duration, ticks chan struct{}, want time.Duration) {
	t.Helper()
	select {
	case got := <-waits:
		if got != want {
			t.Fatalf("poll delay = %s, want %s", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("poll loop did not wait")
	}
	ticks <- struct{}{}
}

func eventually(t *testing.T, f func() bool) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !f() {
		select {
		case <-deadline:
			t.Fatal("condition not reached")
		case <-ticker.C:
		}
	}
}

func TestWebAuthorizationUsesRequestedScopesAndHonorsPollBackoff(t *testing.T) {
	var polls atomic.Int32
	c, waits, ticks, root := fixture(t, func(r *http.Request) (*http.Response, error) {
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/hduhelp-neo/open-apis/auth/device-authorization":
			if r.Method != "POST" || r.Form.Get("client_id") != "hduhelp-cli" || r.Form.Get("scope") != requestedScope || r.Form.Get("device_name") != "HDU Station" || r.Form.Get("device_id") == "" || r.Header.Get("Authorization") != "" || r.Form.Get("supersedes_id") != "" {
				t.Error("unexpected permission, identity or replacement request")
			}
			return response(200, deviceJSON), nil
		case "/hduhelp-neo/open-apis/authen/access-token":
			if r.Method != "POST" || r.Form.Get("device_code") != "device-private-sentinel" || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" || r.Header.Get("x-device-id") == "" {
				t.Error("device poll contract mismatch")
			}
			switch polls.Add(1) {
			case 1:
				return response(400, `{"error":"authorization_pending"}`), nil
			case 2:
				return response(400, `{"error":"slow_down"}`), nil
			case 3:
				return response(503, "private-sentinel"), nil
			default:
				return response(200, tokenJSON), nil
			}
		default:
			t.Error("unexpected write operation")
			return response(500, ""), nil
		}
	})
	opened := ""
	login, err := c.Begin(context.Background(), func(url string) error { opened = url; return nil })
	if err != nil || login.Status != "waiting" || login.UserCode != "ABCD-EFGH" || opened != approvalURL+"?user_code=ABCD-EFGH" {
		t.Fatal("web authorization not started")
	}
	for i := 0; i < 10; i++ {
		c.Poll(login.ID)
	}
	if polls.Load() != 0 {
		t.Fatal("UI polling bypassed advertised interval")
	}
	for _, delay := range []time.Duration{5, 5, 10, 15} {
		advance(t, waits, ticks, delay*time.Second)
	}
	eventually(t, func() bool { return c.Poll(login.ID).Status == "ready" })
	public, _ := json.Marshal(c.Poll(login.ID))
	if strings.Contains(string(public), "private-sentinel") || strings.Contains(string(public), "device_code") {
		t.Fatal("secret returned to Wails")
	}
	reloaded, err := New(root, "legacy-private-sentinel")
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	token, err := reloaded.AccessToken(context.Background())
	if err != nil || token != "new-private-sentinel" || reloaded.credentials.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatal("approved credential not restored")
	}
	for _, scope := range strings.Fields(requestedScope) {
		if _, err := reloaded.AccessTokenFor(context.Background(), scope); err != nil {
			t.Fatal("approved permission not restored")
		}
	}
	info, _ := os.Stat(filepath.Join(root, "campus-auth.yaml"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("credential file permissions are not private")
	}
	data, _ := os.ReadFile(filepath.Join(root, "campus-auth.yaml"))
	if strings.Contains(string(data), "device-private-sentinel") {
		t.Fatal("pending secret persisted")
	}
	if matches, _ := filepath.Glob(filepath.Join(root, ".campus-auth-*")); len(matches) != 0 {
		t.Fatal("temporary credential files left behind")
	}
}

func TestDeniedExpiredAndMissingScopeDoNotReplaceExistingCredential(t *testing.T) {
	for _, tt := range []struct {
		name, body, status string
		code               int
		revoke             bool
	}{
		{"denied", `{"error":"access_denied"}`, "denied", 400, false},
		{"expired", `{"error":"expired_token"}`, "expired", 400, false},
		{"missing-scope", strings.Replace(tokenJSON, CourseScope, "", 1), "error", 200, true},
		{"old-course-only", strings.Replace(tokenJSON, " "+ScheduleScope, "", 1), "error", 200, true},
		{"missing-favorite-read", strings.Replace(tokenJSON, " "+FavoriteReadScope, "", 1), "error", 200, true},
		{"missing-favorite-write", strings.Replace(tokenJSON, " "+FavoriteWriteScope, "", 1), "error", 200, true},
		{"old-four-scopes", strings.Replace(tokenJSON, " "+FavoriteReadScope+" "+FavoriteWriteScope, "", 1), "error", 200, true},
		{"missing-simulation-read", strings.Replace(tokenJSON, " "+SimulationReadScope, "", 1), "error", 200, true},
		{"missing-simulation-write", strings.Replace(tokenJSON, " "+SimulationWriteScope, "", 1), "error", 200, true},
		{"old-read-only", strings.Replace(tokenJSON, " "+SimulationReadScope+" "+SimulationWriteScope, "", 1), "error", 200, true},
		{"duplicate-scope", strings.Replace(tokenJSON, ScheduleScope, CourseScope, 1), "error", 200, true},
		{"extra-scope", strings.Replace(tokenJSON, CourseScope, CourseScope+" academic:grade:read", 1), "error", 200, true},
		{"malformed", `{"msg":"private-sentinel"}`, "error", 200, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var revoked atomic.Bool
			c, waits, ticks, _ := fixture(t, func(r *http.Request) (*http.Response, error) {
				if r.Method == "DELETE" {
					revoked.Store(true)
					return response(200, `{"code":0}`), nil
				}
				if strings.HasSuffix(r.URL.Path, "/device-authorization") {
					return response(200, deviceJSON), nil
				}
				return response(tt.code, tt.body), nil
			})
			login, err := c.Begin(context.Background(), func(string) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			advance(t, waits, ticks, 5*time.Second)
			eventually(t, func() bool { return c.Poll(login.ID).Status == tt.status })
			if tt.revoke {
				eventually(t, revoked.Load)
			}
			token, _ := c.AccessToken(context.Background())
			if token != "legacy-private-sentinel" {
				t.Fatal("failed authorization replaced previous token")
			}
			if strings.Contains(c.Poll(login.ID).Message, "private-sentinel") {
				t.Fatal("upstream error reflected")
			}
		})
	}
}

func TestCancellationRejectsLateApprovalAndCancelsRemoteDevice(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var cancelled, revoked atomic.Bool
	c, waits, ticks, root := fixture(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/device-authorization"):
			return response(200, deviceJSON), nil
		case strings.HasSuffix(r.URL.Path, "/cancel"):
			cancelled.Store(true)
			return response(200, `{}`), nil
		case r.Method == "DELETE":
			revoked.Store(true)
			return response(200, `{"code":0}`), nil
		default:
			close(entered)
			<-release
			return response(200, tokenJSON), nil
		}
	})
	login, err := c.Begin(context.Background(), func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	advance(t, waits, ticks, 5*time.Second)
	<-entered
	c.Cancel("stale-id")
	if !c.Pending() {
		t.Fatal("stale cancellation canceled active authorization")
	}
	c.Cancel(login.ID)
	close(release)
	eventually(t, revoked.Load)
	if !cancelled.Load() || c.Poll(login.ID).Status != "cancelled" {
		t.Fatal("device authorization not canceled")
	}
	reloaded, _ := New(root, "")
	defer reloaded.Close()
	token, _ := reloaded.AccessToken(context.Background())
	if token != "legacy-private-sentinel" {
		t.Fatal("late approval was saved")
	}
}

func TestSaveFailureRevokesNewTokenAndLogoutNeverRestoresCredential(t *testing.T) {
	var revoked atomic.Bool
	c, waits, ticks, root := fixture(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "DELETE" {
			revoked.Store(true)
			return response(503, "private-sentinel"), nil
		}
		if strings.HasSuffix(r.URL.Path, "/device-authorization") {
			return response(200, deviceJSON), nil
		}
		return response(200, tokenJSON), nil
	})
	login, err := c.Begin(context.Background(), func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	persist := c.save
	c.save = func(credentials) error { return ErrStorage }
	c.mu.Unlock()
	advance(t, waits, ticks, 5*time.Second)
	eventually(t, func() bool { return c.Poll(login.ID).Status == "error" })
	eventually(t, revoked.Load)
	if _, err := c.Logout(context.Background()); err != ErrStorage {
		t.Fatal("local logout failure not reported")
	}
	c.mu.Lock()
	c.save = persist
	c.credentials.Managed = true
	c.mu.Unlock()
	message, err := c.Logout(context.Background())
	if err != nil || !strings.Contains(message, "未能确认") || c.Configured() {
		t.Fatal("remote logout failure restored local credentials")
	}
	reloaded, err := New(root, "legacy-private-sentinel")
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	if reloaded.Configured() {
		t.Fatal("logout fell back to legacy credential")
	}
}

func TestAuthorizationURLValidationAndNoRedirects(t *testing.T) {
	for _, uri := range []string{"http://neo.hduhelp.com/account/tokens/authorize?user_code=ABCD-EFGH", "https://evil.test/account/tokens/authorize?user_code=ABCD-EFGH", approvalURL + "?user_code=WRONG", approvalURL + "?user_code=ABCD-EFGH&next=secret", "https://user@neo.hduhelp.com/account/tokens/authorize?user_code=ABCD-EFGH"} {
		c, _, _, _ := fixture(t, func(*http.Request) (*http.Response, error) {
			return response(200, strings.Replace(deviceJSON, approvalURL+"?user_code=ABCD-EFGH", uri, 1)), nil
		})
		if _, err := c.Begin(context.Background(), func(string) error { t.Error("untrusted authorization URL opened"); return nil }); err == nil {
			t.Fatal("unsafe approval page accepted")
		}
	}
	c, _, _, _ := fixture(t, func(*http.Request) (*http.Response, error) {
		r := response(302, "private-sentinel")
		r.Header.Set("Location", "https://evil.test")
		return r, nil
	})
	if _, err := c.Begin(context.Background(), func(string) error { return nil }); err != ErrResponse {
		t.Fatal("redirected auth response accepted")
	}
	if err := c.http.client.CheckRedirect(nil, nil); err != http.ErrUseLastResponse {
		t.Fatal("redirect policy changed")
	}
}

func TestExpiryShutdownAndPlatformSupport(t *testing.T) {
	c, waits, _, _ := fixture(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			return response(200, `{}`), nil
		}
		return response(200, deviceJSON), nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	login, err := c.Begin(ctx, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	<-waits
	eventually(t, func() bool { return c.Poll(login.ID).Status == "expired" })
	c.mu.Lock()
	c.credentials.ExpiresAt = time.Now().Add(-time.Second).UnixMilli()
	c.mu.Unlock()
	if _, err := c.AccessToken(context.Background()); err != ErrLoginRequired {
		t.Fatal("expired PAT used")
	}
	c.Close()
	if _, err := c.Begin(context.Background(), func(string) error { return nil }); err == nil {
		t.Fatal("login after shutdown accepted")
	}
	for _, os := range []string{"darwin", "windows", "linux"} {
		if !Supported(os) {
			t.Fatal("supported desktop rejected")
		}
	}
	for _, os := range []string{"plan9", "android", "ios", ""} {
		if Supported(os) {
			t.Fatal("unsupported platform advertised")
		}
	}
	if waitForPoll(ctx, time.Hour) {
		t.Fatal("cancelled wait continued")
	}
}

func TestInvalidFilesAndPublicJSONDoNotExposeCredentials(t *testing.T) {
	root := t.TempDir()
	for _, value := range []string{"version: 99\n", "version: 1\ndevice_id: okay\ntoken: 'bad token'\n", "invalid: ["} {
		if os.WriteFile(filepath.Join(root, "campus-auth.yaml"), []byte(value), 0600) != nil {
			t.Fatal("fixture failed")
		}
		if _, err := New(root, ""); err == nil {
			t.Fatal("invalid stored auth accepted")
		}
	}
	data, _ := json.Marshal(credentials{Token: "private-sentinel"})
	if strings.Contains(string(data), "private-sentinel") {
		t.Fatal("credential serialized")
	}
}

func TestOldCourseAuthorizationKeepsCourseReadsAndRequiresScheduleUpgrade(t *testing.T) {
	c, _, _, _ := fixture(t, func(*http.Request) (*http.Response, error) {
		t.Error("permission check made network request")
		return nil, ErrUnavailable
	})
	c.credentials.Managed = true
	c.credentials.Scopes = []string{CourseScope}
	if _, err := c.AccessTokenFor(context.Background(), CourseScope); err != nil {
		t.Fatal("old course grant lost")
	}
	if _, err := c.AccessTokenFor(context.Background(), ScheduleScope); err != ErrScope {
		t.Fatal("personal schedule read bypassed permission upgrade")
	}
	if c.HasScope(ScheduleScope) {
		t.Fatal("old grant advertised new permission")
	}
	c.credentials.Scopes = []string{CourseScope, ScheduleScope}
	if _, err := c.AccessTokenFor(context.Background(), ScheduleScope); err != nil {
		t.Fatal("approved schedule grant denied")
	}
}
