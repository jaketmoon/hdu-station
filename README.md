# HDU Station

HDU Station 是安装在学生个人电脑上的本地校园 AI 工作站。它通过一个统一的对话入口连接校园能力、腾讯频道和公开网页，并在需要执行代码时自动进入 Station 自带的隔离环境。

## 产品边界

- 本地优先：配置、会话、缓存和工作区默认只保存在用户电脑。
- 单一入口：用户不需要选择 Agent 或切换 Sandbox 模式。
- 只读校园助手：首发只查询校园、课程和公开信息，不执行选课等业务写操作。
- 自动隔离：自由 Shell、Python、Node 和模型生成代码永远不在宿主机执行。
- 轻量安装：桌面主程序不内置 Linux 镜像，Sandbox 和工具包按需下载。
- 可彻底清理：应用可以清除自己创建的配置、数据库、缓存、工作区和 Sandbox。

## 首发平台

- Windows 10/11 x64
- macOS 13+ arm64

## 技术栈

- Wails 2 + Go
- React + TypeScript + Vite
- YAML 本地配置
- SQLite 本地会话存储
- macOS Virtualization.framework/vfkit Sandbox（vsock runtime）
- Windows WSL2 专用发行版 Sandbox（关闭驱动挂载与 Windows interop）

## 模型与工具

- OpenAI Chat Completions
- OpenAI Responses API
- Anthropic Messages API
- HDUHelp Neo MCP（宿主侧 PAT、校园只读能力）
- 腾讯频道 CLI（指定频道只读检索）
- Web Search / Web Fetch

模型支持 SSE 增量响应；桌面 UI 通过本地 Wails 事件逐字显示，工具调用仍会经过同一个有界的只读工具循环。

选课相关规则由版本化的 `course-selection` Skill 提供。Skill 内的社区频道候选不会进入全局配置；“水课”“给分”等只作为腾讯频道社区信号，不会被 Station 当作校园事实。

## 当前开发前置条件

开发版不会内置 Sandbox 镜像。要启用隔离执行，需要通过 `.env` 或本地配置提供平台匹配的 HTTPS 镜像 URL 和 SHA-256；正式发布还必须提供签名镜像及发布元数据。缺少这些信息时，Station 会显示不可用并拒绝执行，不会回退到宿主机。

腾讯频道的课程 Skill 只允许其中三个已验证的杭电数字 guild ID；官方 CLI 登录凭据由腾讯 CLI 管理，Station 只注册频道内只读检索，不会猜测频道或调用任意频道。

详细设计见 [docs/architecture.md](docs/architecture.md)，分阶段实施和验收证据见 [docs/roadmap.md](docs/roadmap.md)。

## 开发状态

项目正在从初始全栈服务模板逐步改造成桌面应用。每个阶段独立提交并附带验证，最终用户不需要预装 Docker、Python 或 Node。
