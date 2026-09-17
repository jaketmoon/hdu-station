# HDU Station Agent Instructions

## Before changing code

1. 运行 `git status --short --branch`，保留用户已有改动。
2. 阅读本文件和 `docs/architecture.md`。
3. 检查改动属于桌面壳、Agent、工具、Skill、Sandbox、存储还是打包边界。
4. 优先完成一个可运行的纵向切片，不提前搭建没有调用方的抽象。
5. 从 hdu-mate 复用代码时按文件选择性复制并立即去除服务端、多租户和 Shared Runner 假设。

## Architecture rules

- React 只调用 Wails 绑定，不依赖远端 Station 后端。
- Go composition root 统一装配配置、存储、模型、工具和 Sandbox。
- 模型供应商差异收敛在 provider 适配器，Agent 不分叉业务逻辑。
- Neo、腾讯频道和 Web Search 默认只读；课程收藏和 Neo 模拟课表可按用户明确要求增删改查，必须核实教学班、保留未指定修改的内容并复查结果，模拟方案必须校验 revision。学校真实加退课与其他业务写命令不得注册。
- 任意 Shell 或用户代码只能通过 `sandbox.Executor` 执行，禁止宿主机 fallback。
- API Key 和校园 Key 按产品约定保存到 YAML；日志和测试输出不得回显凭证。
- 三个杭电腾讯频道只能出现在选课 Skill 内，不进入全局配置。
- Station 创建的持久数据必须位于单一应用数据根目录，并纳入清理清单。
- 不引入 PostgreSQL、Redis、Docker、Kubernetes 或线上租户权限系统。

## Verification

提交前根据改动至少运行：

```bash
make test
make frontend-check
make build
```

涉及界面的改动还需检查桌面和窄窗口布局；涉及平台代码时必须有平台能力检测和不支持状态测试。

