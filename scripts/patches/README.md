# 小红书服务的本机兼容补丁

`xiaohongshu-mcp-v2.5.0.patch` 对应上游 `v2.5.0`，提交 `6583124dfda92312b6bc19a042a6acfae63fe498`，沿用上游 Apache-2.0 许可，见 `docs/third-party/xiaohongshu-mcp-LICENSE`。

补丁包含两项本机适配：允许 `XHS_BROWSER_CACHE_DIR` 指定绝对缓存目录；搜索等待实际搜索状态和结果，而不是等待整页网络、DOM 停止变化。后者修复结果已经加载但后台活动持续导致约 60 秒超时的问题，并区分初始加载中的空数组、已完成的空结果及加载失败。

在对应上游源码目录应用补丁后重新编译主程序，保留原有 Cookie、服务 Token 和启动参数。示例：

```sh
git apply /path/to/hdu-station/scripts/patches/xiaohongshu-mcp-v2.5.0.patch
go build -ldflags '-X main.version=v2.5.0-station-local' -o ../xiaohongshu-mcp .
```

回归测试只访问本机合成页面，不使用账号或访问小红书。显式设置已有浏览器路径后运行：

```sh
XHS_TEST_BROWSER_BIN=/absolute/path/to/Chromium go test ./xiaohongshu -run '^TestSearchDataReadiness$' -v -count=1 -timeout=60s
```

覆盖持续变化的页面、加载中的空数组、成功的空结果、失败状态和取消操作。未指定浏览器路径时跳过，不自动安装浏览器。升级上游版本时需重新核对搜索状态字段，不能直接忽略补丁冲突。


2026-09-13：补丁新增搜索安全验证的设置接口。搜索跳转到 `/website-login/captcha` 时立即返回 HTTP 428，登录状态额外返回 `verification_required`；保留触发验证的浏览器最多 50 秒。设置通过固定 `/security/qrcode`、`/security/poll`、`/security/cancel` POST 接口取本站二维码、检查实际搜索恢复并取消等待。会话只在内存中；二维码、会话 URL 和 Cookie 不进入模型与日志。扫码后必须实际搜索成功才保存 Cookie 并显示恢复；二维码过期重新发起，不能复用普通登录二维码。旧版服务不支持时设置明确提示更新组件。
