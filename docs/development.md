# 开发

需要 Go 1.25+、Node.js 20.19+ 和目标系统的 Wails 2 编译依赖。

```sh
make frontend-install
make test
make build
```

`frontend/src/api.ts` 是前端唯一的宿主边界。使用 `wails dev` 可运行完整桌面开发环境；单独 `npm run dev` 可查看界面，但真实问答只能由 Wails 提供，不会转到远端后端或返回演示数据。

`make e2e` 使用已安装的 Google Chrome，在测试文件里注入 Wails 替身，检查 1280×820 与 390×844。截图写入 `frontend/test-results/`。单测覆盖中文输入法、对话切换、流式停止、配置保留、SQLite 恢复、SSE 截断和工具作用域。

`make live-test` 是显式的真实联网验收；未设置 `HDU_STATION_LIVE_TEST=1` 时自动跳过。不要在日志、截图或测试失败信息中输出凭证。

新增来源可先独立联调，再检查完整问答。启用并登录后，运行 `HDU_STATION_LIVE_SOURCE=xiaohongshu go test ./internal/tools -run '^TestLiveSourceSearchAndRead$' -v -count=1 -timeout=5m`。这会真实搜索“杭电 选课”并读取前三篇笔记，可增加 `HDU_STATION_LIVE_QUERY=高数` 复现指定问题，不调用模型或平台写接口。报告写入数据根目录 `logs/live-source-xiaohongshu.json`，只保留数量、公开原帖地址、文字长度和完整性标记；日常测试默认跳过。赞哦可用 `HDU_STATION_LIVE_SOURCE=zanao` 单独检查，需要先配置学校和 Token。

小红书请求错误的固定类别和耗时见 `logs/xiaohongshu-requests.jsonl`。搜索页等待修复及本机浏览器回归用例保存在 [上游兼容补丁](../scripts/patches/README.md)，重装或升级独立服务时需核对。连接适配器测试覆盖一次性重试、认证和限流不重试、取消、错误分类和诊断脱敏。

源码结构：

- `app.go`：Wails 用例、单轮生命周期和取消。
- `source_settings.go`：来源开关、二维码登录、会话轮询与凭证清除。
- `campus_settings.go` / `internal/campusauth`：校园网页授权、受限 Device Flow 和私有 YAML 凭证。
- `internal/agent`：Eino 循环和 DeepSeek SSE 适配。
- `internal/tools`：多来源搜索会话、只读 QQ/赞哦/小红书适配器和 QQ 完整性安装器。
- `internal/skills`：简短选课提示与唯一的频道范围数据。
- `internal/config` / `internal/storage`：私有 YAML 与本机对话历史。
- `frontend/src`：聊天界面、设置、Markdown 渲染和绑定契约。

没有通用 Agent、复杂证据账本、动态技能安装、Sandbox 或线上租户依赖。

可选来源的配置与上游接口版本见 [搜索来源配置](search-sources.md)。Go 测试通过本机 HTTP 替身检查赞哦签名、两种搜索返回结构、小红书鉴权和搜索后读帖、多平台引用、失败隔离、取消与凭证脱敏；Eino 测试覆盖小红书搜索到可见引用的完整工具链。`make e2e` 还检查来源折叠卡片、即时保存开关、二维码展示、模拟扫码成功、清除凭证及两种窗口下的滚动布局。这些替身测试不代表真实账号已经登录或通过平台风控。

校园网页授权测试覆盖批准、拒绝、过期、慢轮询、取消后的迟到结果、存储失败、退出后不回退旧令牌、URL 校验与凭证脱敏。`go test -race ./internal/campusauth .` 检查授权与桌面用例的并发访问。`HDU_STATION_LIVE_CAMPUS_AUTH=1 go test ./internal/campusauth -run TestLiveCampusDeviceAuthorization -v -count=1` 仅创建并取消一条未批准的真实授权请求，不打开网页或领取 PAT，不输出设备码或授权码。桌面与窄窗口 E2E 验证 HDU CLI 登录卡片、网页等待、本机状态更新及退出授权。

课程分类查询已移除。回归检查 Agent 只注册社区搜索与读帖工具、登录状态不访问业务接口，以及 SQLite 从版本 1/2 升级至 3 后清理旧缓存但保留对话记录。
