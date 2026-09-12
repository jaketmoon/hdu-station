package campusauth

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Login struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	UserCode  string `json:"userCode,omitempty"`
	ExpiresAt int64  `json:"expiresAt"`
	Message   string `json:"message,omitempty"`
}

type attempt struct {
	Login
	device   deviceResponse
	deviceID string
	ctx      context.Context
	cancel   context.CancelFunc
}

func formatMillis(t time.Time) string { return strconv.FormatInt(t.UnixMilli(), 10) }

func (c *Client) Begin(ctx context.Context, openBrowser func(string) error) (Login, error) {
	if openBrowser == nil {
		return Login{}, errors.New("当前环境无法打开官方授权页")
	}
	c.cancel("")
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return Login{}, ErrUnavailable
	}
	deviceID := c.credentials.DeviceID
	if c.save(c.credentials) != nil {
		c.mu.Unlock()
		return Login{}, ErrStorage
	}
	c.mu.Unlock()
	device, err := c.http.begin(ctx, deviceID)
	if err != nil {
		return Login{}, err
	}
	loginCtx, cancel := context.WithTimeout(ctx, time.Duration(device.ExpiresIn)*time.Second)
	deadline, _ := loginCtx.Deadline()
	a := &attempt{Login: Login{ID: uuid.NewString(), Status: "waiting", UserCode: device.UserCode, ExpiresAt: deadline.UnixMilli()}, device: device, deviceID: deviceID, ctx: loginCtx, cancel: cancel}
	c.mu.Lock()
	if c.closed || loginCtx.Err() != nil {
		c.mu.Unlock()
		cancel()
		c.cancelRemote(a)
		return Login{}, ErrUnavailable
	}
	c.attempt = a
	result := a.Login
	c.mu.Unlock()
	if err := openBrowser(device.URL); err != nil {
		c.cancel(a.ID)
		return Login{}, errors.New("无法打开官方授权页，请重新连接")
	}
	go c.awaitAuthorization(a)
	return result, nil
}

func waitForPoll(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}

func (c *Client) awaitAuthorization(a *attempt) {
	defer a.cancel()
	interval := time.Duration(a.device.Interval) * time.Second
	for c.wait(a.ctx, interval) {
		token, err := c.http.poll(a.ctx, a.device.DeviceCode, a.deviceID)
		if a.ctx.Err() == nil {
			switch err {
			case deviceError("authorization_pending"):
				continue
			case deviceError("slow_down"):
				interval += 5 * time.Second
				continue
			case ErrUnavailable:
				if interval < 30*time.Second {
					interval += 5 * time.Second
				}
				continue
			}
		}
		c.mu.Lock()
		accepted := !c.closed && c.attempt == a && a.ctx.Err() == nil && a.Status == "waiting"
		old := c.credentials
		if accepted {
			switch err {
			case deviceError("expired_token"):
				a.Status = "expired"
			case deviceError("access_denied"):
				a.Status = "denied"
			case nil:
				// Missing scope never means approval of the requested permission.
				if strings.TrimSpace(token.Scope) != CourseScope {
					a.Status, a.Message = "error", "未获得课程查询权限，请重新授权并保留“读取课程信息”。"
				} else {
					next := credentials{Version: 1, DeviceID: a.deviceID, Token: token.Token, Scopes: []string{CourseScope}, Managed: true}
					if token.ExpiresIn > 0 {
						next.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UnixMilli()
					}
					if c.save(next) != nil {
						a.Status, a.Message = "error", ErrStorage.Error()
					} else {
						c.credentials = next
						a.Status = "ready"
					}
				}
			default:
				a.Status, a.Message = "error", "校园授权未完成，请重新连接。"
			}
		}
		saved := accepted && a.Status == "ready"
		c.mu.Unlock()
		if err == nil {
			if !saved {
				c.revoke(token.Token)
			} else if old.Managed && old.Token != "" && old.Token != token.Token {
				c.revoke(old.Token)
			}
		}
		if !accepted {
			c.finishExpired(a)
		}
		return
	}
	c.finishExpired(a)
}

func (c *Client) revoke(token string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.http.revoke(ctx, token)
}

func (c *Client) finishExpired(a *attempt) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if a.Status == "waiting" {
		a.Status = "cancelled"
		if errors.Is(a.ctx.Err(), context.DeadlineExceeded) {
			a.Status = "expired"
		}
	}
}

func (c *Client) Poll(id string) Login {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.attempt == nil || c.attempt.ID != id {
		return Login{ID: id, Status: "cancelled"}
	}
	return c.attempt.Login
}

func (c *Client) Pending() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.attempt != nil && c.attempt.ctx.Err() == nil && c.attempt.Status == "waiting"
}

func (c *Client) Open(id string, openBrowser func(string) error) error {
	c.mu.Lock()
	a := c.attempt
	if a == nil || a.ID != id || a.ctx.Err() != nil || a.Status != "waiting" {
		c.mu.Unlock()
		return errors.New("授权请求已结束，请重新连接")
	}
	address := a.device.URL
	c.mu.Unlock()
	return openBrowser(address)
}

func (c *Client) cancelRemote(a *attempt) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.http.cancel(ctx, a.device.DeviceCode, a.deviceID)
}

func (c *Client) cancel(id string) {
	c.mu.Lock()
	a := c.attempt
	if a == nil || (id != "" && a.ID != id) || a.Status != "waiting" {
		c.mu.Unlock()
		return
	}
	a.Status = "cancelled"
	a.cancel()
	c.mu.Unlock()
	c.cancelRemote(a)
}

func (c *Client) Cancel(id string) {
	if id != "" {
		c.cancel(id)
	}
}
