# Project Agent Instructions

这是一个契约驱动、按业务上下文组织的全栈项目。

## Before changing code

1. 运行 `git status --short --branch`，保留用户已有改动。
2. 阅读目标目录最近的 `AGENTS.md` 和 `docs/architecture.md`。
3. 先定位所属 bounded context，再沿完整请求链路修改。
4. API 改动先修改 `idl/`；数据模型改动先修改 `types/dbmodel/`。
5. 生成代码后检查 diff，不要无条件覆盖用户的手写业务逻辑。

## Architecture rules

- 依赖方向：`api -> application -> domain`。
- `infra` / `internal` 实现端口，由 composition root 装配。
- handler 不直接查库，不把业务规则塞进 React 页面。
- 前端 API DTO 必须来自生成的 OpenAPI 类型。
- 所有响应遵守统一 envelope：`{ data, code, msg }`。
- 所有新功能必须覆盖 loading、empty、error、success 和权限失败状态。

## Vibe Coding delivery

每次只实现一个可验证的纵向功能切片：契约、后端用例、数据访问、前端交互、测试和浏览器验证一起完成。

完成前至少运行：

```bash
make test
cd frontend/app && pnpm run check
cd frontend/admin && pnpm run build
```

前端功能还必须在移动端和桌面端真实浏览器中验证。

