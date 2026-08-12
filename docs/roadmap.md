# HDU Station implementation roadmap

这份路线图把当前代码状态和产品发布前必须补齐的外部验收分开记录。状态只在有对应证据时标记为“完成”；单元测试不能替代真实平台、真实账号或正式发布验收。

## 当前结论

Station 已经具备一个可运行的本地纵向切片：Wails + React 桌面壳、YAML/SQLite 本地存储、provider-neutral Agent、SSE 流式响应、Neo MCP 校园只读工具、腾讯频道只读工具边界、Web 工具，以及不允许宿主机 fallback 的 Sandbox 接口和平台后端骨架。

当前版本适合继续做集成验收，不适合宣称“可发布的完整产品”。

## 分阶段计划

| 阶段 | 目标 | 当前状态 | 进入下一阶段的证据 |
| --- | --- | --- | --- |
| 0. 边界与决策 | 固定本地优先、Wails、SQLite、只读工具和全 Sandbox 原则 | 完成 | 架构文档、配置/存储/工具边界测试 |
| 1. 桌面纵向切片 | 对话、会话持久化、模型协议适配、工具审计、设置页 | 代码完成；本机验证通过 | `make test`、`make frontend-check`、`make build`，并通过桌面/320px 布局检查 |
| 2. 校园与社区连接器 | Neo PAT 真实读取；腾讯 CLI 登录、频道索引和三条选课检索 | Neo 与腾讯真实只读验证完成；Station 的正式下载/安装路径有单元测试 | Neo 真实只读成功；腾讯真实登录成功；三个真实数字频道 ID 只写入课程 Skill；分别检索“选课/选好课/选水课” |
| 3. Sandbox POC | macOS vfkit 和 Windows WSL2 都能安装、启动、执行、停止、清理 | 代码和协议测试完成；实机未验收 | 每个平台用真实镜像完成一次成功执行、超时、越界、清理和无宿主 fallback 验证 |
| 4. 选课助手 | 用 Neo 官方事实 + 腾讯社区信号 + 网页证据生成有来源、不夸大的课程比较 | `course-selection` Skill 0.1.2 规则与 Neo/腾讯只读数据源均已真实验证 | 三个选课场景有可复核来源、冲突/缺失信息和“不确定性”输出；不执行选课写操作 |
| 5. 发布工程 | 签名镜像、vfkit/腾讯 CLI 许可、Windows 安装包、macOS 签名公证、更新和回滚 | 未完成 | 干净机器安装/升级/卸载演练；签名验证、清理清单、版本元数据和许可证审查全部留档 |

## 当前未决事项与处理方式

### Neo 登录

当前桌面端只接受用户输入的校园 PAT，并把 PAT 保留在宿主侧 YAML；课程检索、本人选课和课表分别需要 `academic:course:read`、`academic:studentselection:read`、`academic:schedule:read`。Station 会把缺少 scope 与 Key 被拒绝分开显示。`.env` 只在创建 Station `config.yaml` 时作为开发种子，不会覆盖既有 Station 配置，也不会自动读取、复制或覆盖 hduhelp-cli 的独立 PAT。正式 Device Authorization 不冒充现有 `hduhelp-cli` 客户端；要做桌面登录，服务端必须先注册并公开 `hdu-station` 公共客户端及其授权契约。注册完成前，UI 应继续显示“使用本机 Key”，不能显示一个看似可用的设备登录按钮。

### 腾讯频道

腾讯 CLI 的官方登录状态由 CLI 自己管理，Station 不读取或展示 token/state 文件。当前已确认 CLI 凭据在系统钥匙串中有效，服务连通正常；已加入频道中仅将三个杭电频道写入课程 Skill，登录后只做 `get-my-join-guild-info` 和频道内 `search-guild-feeds` 只读调用。

频道 ID 只属于课程 Skill，不进入全局配置；“笔记本电脑”虽已加入，也不属于选课检索范围。

### Sandbox 交付

开发版本没有默认镜像 URL、SHA-256、签名和公钥。镜像发布服务需要先提供按平台的签名 artifact 与元数据；客户端随后验证 HTTPS、大小、SHA-256、Ed25519 签名和相对路径，再安装 guest runtime。缺少镜像时必须保持不可用，不得通过宿主 Shell 降级。

macOS 还需要确认 vfkit 的再分发许可；Windows 需要在真实 Windows 10/11 x64 上验证 WSL2 专用发行版导入、硬化、ownership marker 和卸载行为。

## 本轮验收证据

- `make test`：使用 `/tmp` 隔离 Go 缓存后通过。
- `make frontend-check`：TypeScript 检查和 Vite 构建通过。
- `make build`：使用 `/tmp` 隔离 Go 缓存后通过。
- `go vet ./...`：使用 `/tmp` 隔离 Go 缓存后通过。
- `make sandbox-runtime`：Linux amd64/arm64 runtime 构建通过。
- Darwin arm64 的 Sandbox/Tools 测试通过；Windows 交叉编译只能证明可编译，不能证明 Windows 实机行为。
- 安全边界复审补充了宿主侧 NUL 拒绝、Agent 工具调用/参数/结果/轮数上限，以及模型 HTTP/SSE 客户端禁止重定向；相关回归测试通过。
- 课程 Skill 的腾讯频道索引超过三个数字 guild ID 时 fail-closed；频道搜索结果在进入 Agent 前剥离凭证、分页状态、内部对象 ID、原始对象和直接身份字段，并拒绝非单值 JSON；相关回归测试通过。
- Agent 在每次 provider 请求前注入不可变的外部证据安全边界：校园、腾讯频道和网页内容不能升级为系统指令、扩大工具权限、绕过 Sandbox 或触发写操作；相关回归测试通过。
- Sandbox Unix-socket 客户端现在严格读取单个有界 NDJSON 响应并拒绝同一行尾随数据；guest runtime 协议异常不会被当作成功执行；相关回归测试通过。
- Sandbox Unix-socket 调用现在在请求后半关闭写端，并拒绝连接上的第二条响应；Windows WSL stdio 复用同一严格解码路径。macOS/Windows Sandbox 包分别有 arm64/amd64 交叉编译产物证据，但这不替代真实平台执行验收。
- Windows WSL 控制命令和 guest runtime 具有默认超时；发行版查询失败时 fail-closed，App 清理在 Sandbox purge 失败时不会删除 Station 数据。相关回归测试和 Windows 交叉编译通过。
- Sandbox 镜像 artifact 校验现在拒绝符号链接路径并复核解析后的路径 containment，避免本地清单或目录被篡改后把 Sandbox 外部文件当作已验证镜像；相关回归测试通过。
- macOS vfkit 后端现在只有在 guest runtime 的 `ping` 成功后才报告 Ready 或接受执行；进程已启动但 runtime 尚未就绪时 fail-closed，相关 Darwin 回归测试通过。
- 本地 UI 在 1280×720 和 320×720 下无横向溢出；窄窗口快捷问题按钮与输入框重叠问题已修复。
- Neo MCP 的真实 `academic class search` 已用 hduhelp-cli 当前有效 PAT 成功完成；Station 也已通过显式导入将该 PAT 保存到自己的本地配置后完成同一只读查询。Station 的请求已按 Neo 生成契约把参数写入 `arguments.query`；不带参数的 `schedule.now` 保持空参数对象。真实响应正文和 PAT 均未写入仓库、日志或回复。
- Neo 适配器按工具声明最小权限，并对 JSON-RPC/MCP 工具级错误做安全分类；上游错误文本不会进入 Agent。缺少权限时会提示对应 scope，invalid token 时显示为凭据拒绝；参数位置分组与无参数工具的 wire 形状均有回归测试。
- 腾讯 CLI 已通过真实钥匙串状态检查，服务连通正常；三个杭电频道的“选课 / 选好课 / 选水课”只读检索均已完成，结果未写入仓库，原始身份字段和分页令牌未进入 Agent 或 Skill。
- `course-selection` Skill 0.1.2 已明确现有 Neo、三个真实杭电腾讯频道和 Web 工具的调用顺序、缺失/冲突处理和固定输出区块；没有新增选课专用代码层。

## 下一步顺序

1. 为正式产品向 Neo 服务端申请/注册 `hdu-station` Device Authorization 客户端；当前 PAT 手动配置路径已经过真实只读验收。
2. 生成并签名 macOS arm64 与 Windows amd64 Sandbox 镜像，在对应平台完成实机 POC。
3. 再开始安装包、签名、公证、更新器和许可证发布工作。

## 不可破坏的安全验收条件

- 任意 Shell 或用户代码只能经过 `sandbox.Executor`；Sandbox 不可用时请求失败。
- Neo PAT、模型 API Key、腾讯登录凭据不出现在 UI DTO、日志、审计详情或 Sandbox 输入中。
- Agent 工具注册只包含明确的只读校园/社区/网页能力；选课、发帖、评论、成员管理等写操作不注册。
- Station 只清理自己创建的数据和自己拥有的 WSL 发行版，不删除共享 OS 组件或外部发行版。
