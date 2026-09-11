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
