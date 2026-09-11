# HDU Station：选课助手

## 范围

启动即进入选课对话。使用 Go、Wails 2、React 19、TypeScript、Eino、SQLite 和 YAML。没有远端 Station 后端，没有通用校园模式、Agent 选择、附件、任意 Shell 或 Sandbox。

桌面旧项目只用于理解接口与复用连接环境；本次重新实现聊天、模型适配、工具组合、存储和前端。保留当前仓库已有的带校验 QQ CLI 安装器。

## 调用链

```text
React → Wails App.Chat → Eino ReAct → DeepSeek V4.1 Flash
                              ↕
                    search_courses / read_course_posts
                              ↕
                       只读 QQ 频道 CLI
```

`App` 是唯一 composition root，装配配置、SQLite、模型与工具。一次只允许一个回答在运行；每个请求有独立取消上下文。用户可以在生成过程中切换历史，事件按请求与对话隔离。停止、超时和失败保留已显示的部分回答，重启时把未完成记录标记为中断。

## 模型与 Skill

`internal/agent/provider.go` 实现 Eino ToolCallingChatModel。模型接口使用 Chat Completions SSE，禁止带凭证重定向，不把上游错误原文返回界面。正式 DeepSeek API 使用 `deepseek-flash`；迁移网关使用其目录中的 `deepseek-v4.1-flash`。实测网关返回模型名 `deepseek-flash`。

只有一个内置 Skill：`internal/skills/course-selection/SKILL.md`。正文允许 Agent 自由选择关键词与自然回答形式，不要求 JSON、固定表格、课程数量或反复闭包检索。三个频道 ID 只保存在 Skill 的作用域区，加载后注入宿主工具，不发送给模型。

模型偶尔会引用本轮帖子编号，宿主将它解析成已读原帖链接，流式分片也使用同一转换。追问可以沿用历史完整回答中已验证的链接；本轮读过帖子却没有引用时，在回答末尾补充来源，不要求固定回答结构。

Eino 控制搜索与读帖循环。每轮最多读取 12 个不同帖子，工具调用预算和最多六轮带工具模型请求限制总等待时间；之后保留一次无工具汇总。整轮最多六分钟，每个模型请求最多 90 秒，CLI 请求最多 25 秒。预算是运行时限制，不是回答质量或格式门槛。

## 工具

- `search_courses`：一个简短关键词覆盖三个频道，返回标题、日期、评论数及本轮帖子引用。每个频道最多 20 条；部分频道失败不丢弃其他结果。
- `read_course_posts`：一次读取最多六个已搜索到的引用，取得正文、原帖链接、最多两页评论及回复预览。截断和读取缺口显式标记，重复引用复用本轮结果。

模型不能提供 guild ID、CLI 命令、Shell、登录令牌或任意 URL。CLI 参数由宿主固定构造，没有解释器。输出只提取课程研究需要的文本和公开链接；作者标识、凭证和游标不进入模型或 SQLite。QQ 限流会等待 70 秒重试一次，可随时取消。安装仅使用仓库已有的固定版本及完整性校验，不从 PATH 猜测二进制。

## 本机数据与迁移

新版使用独立应用数据根目录 `HDU Station Course`，避免覆盖旧版配置、数据库和技能。在 macOS 下为 `~/Library/Application Support/HDU Station Course`。开发或测试可显式设置 `HDU_STATION_DATA_ROOT`。

清理清单：`config.yaml`、`station.db` 及 WAL/SHM、`tools/`、`logs/`。`config.OwnedEntries` 记录同一清单。关闭应用后删除此目录即可移除全部新版持久数据。QQ CLI 使用原有 `~/.qqcli` 登录，该凭证不归应用所有，不随清理删除。

模型 Key 和迁移的校园 PAT 保存在权限为 0600 的 YAML。PAT 暂存用于后续用户明确需要的功能，当前选课助手不使用。界面只知道凭证是否已配置。SQLite 仅保存用户问题和可见回答，不保存原始工具内容。未来数据库/配置版本直接拒绝，不尝试降级覆盖。

`cmd/import-config` 是显式执行的一次性连接配置迁移工具，不在产品启动时读取 `.env`，也不自动扫描旧项目。模型凭证和 QQ 环境已按用户授权迁移，代码与凭证不会混入发布包。

## 验证

`make test`、`make frontend-check`、`make build` 检查单测、类型和桌面构建。`make e2e` 在桌面与 390px 窄窗口检查布局、对话、停止及设置；测试替身只存在于测试文件。

`make live-test` 显式使用本机真实配置，依次发送三个选课问题，要求实际 DeepSeek 模型、QQ 搜索、读帖、流式文本和可追溯原帖链接。脱敏可见回答写入数据根目录 `logs/acceptance.md`，用于人工核对内容。日常单测跳过这些联网调用。
