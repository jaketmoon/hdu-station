package tools

import (
	"context"
	"errors"
	"time"
)

// Settings-only: security QR images and session handles never reach the Agent.
type SecurityChallenge struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Image     string `json:"image,omitempty"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
}

func (c *XiaohongshuClient) BeginVerification(ctx context.Context) (SecurityChallenge, error) {
	var r SecurityChallenge
	if err := c.request(ctx, "/api/v1/security/qrcode", struct{}{}, &r); err != nil {
		if errors.Is(err, errSourceResponse) {
			return r, errors.New("本机小红书组件不支持安全验证或响应无效，请更新连接组件")
		}
		return r, sourceError("小红书", err)
	}
	if r.Status == "ready" {
		r.Image = ""
		return r, nil
	}
	if r.Status != "waiting" || len(r.ID) != 32 || r.ExpiresAt <= time.Now().UnixMilli() || r.ExpiresAt > time.Now().Add(time.Minute).UnixMilli() {
		return SecurityChallenge{}, errors.New("安全验证二维码已过期，请重新获取")
	}
	img, err := loginImage(r.Image)
	if err != nil {
		return SecurityChallenge{}, err
	}
	r.Image = img
	return r, nil
}
func (c *XiaohongshuClient) PollVerification(ctx context.Context, id string) (string, error) {
	var r SecurityChallenge
	if err := c.request(ctx, "/api/v1/security/poll", map[string]string{"id": id}, &r); err != nil {
		return "", sourceError("小红书", err)
	}
	switch r.Status {
	case "waiting", "ready", "expired", "cancelled":
		return r.Status, nil
	}
	return "", errors.New("安全验证状态无效")
}
func (c *XiaohongshuClient) CancelVerification(ctx context.Context, id string) {
	var r struct{}
	_ = c.request(ctx, "/api/v1/security/cancel", map[string]string{"id": id}, &r)
}
