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
- macOS Virtualization.framework/vfkit Sandbox
- Windows WSL2 专用发行版 Sandbox

## 模型与工具

- OpenAI Chat Completions
- OpenAI Responses API
- Anthropic Messages API
- HDUHelp Neo CLI（校园只读能力）
- 腾讯频道 CLI（指定频道只读检索）
- Web Search / Web Fetch

详细设计见 [docs/architecture.md](docs/architecture.md)。

## 开发状态

项目正在从初始全栈服务模板逐步改造成桌面应用。每个阶段独立提交并附带验证，最终用户不需要预装 Docker、Python 或 Node。
