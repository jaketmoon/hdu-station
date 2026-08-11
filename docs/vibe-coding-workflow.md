# Vibe Coding Workflow

## 开始前

把需求写成一个用户动作，而不是一个技术任务：

```text
用户是谁？
要完成什么动作？
成功后看到什么？
没有数据、没有权限、上游失败时怎么办？
```

然后确定：bounded context、认证类型、数据来源、是否需要新表、是否需要新 API。

## 实现顺序

1. 阅读目标上下文的 IDL、application、domain repository 和前端 service。
2. 先修改 IDL 或 dbmodel 事实源。
3. 运行生成命令并检查生成 diff。
4. 实现 domain 端口和 application 用例。
5. 在 composition root 装配新依赖。
6. 实现 handler 的认证、参数转换和响应映射。
7. 更新前端 service，再实现页面和状态。
8. 用真实本地数据走一次浏览器链路。

## 生成命令

```bash
make hz
make gen-dal
cd frontend/app && pnpm gen:api
cd frontend/admin && pnpm gen:api
```

## Agent 不应做的事

- 不根据页面猜测 API 字段。
- 不在前端临时复制 DTO 类型。
- 不在 handler 中写数据库查询。
- 不用 fake 数据掩盖核心数据源不可用。
- 不为了一个小功能重构全局架构。
- 不把生成文件的大范围变化混入业务改动而不说明原因。

## 验收清单

- API 契约和两个前端类型已同步。
- 成功、空数据、参数错误、未登录、无权限、上游失败状态已处理。
- 数据库查询使用结构化生成 API。
- 单元测试覆盖业务分支。
- 前端在移动端和桌面端完成浏览器检查。
- `git diff` 不包含无关生成物或调试文件。

