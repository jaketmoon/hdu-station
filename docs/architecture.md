# Architecture Contract

## 1. 分层

```text
IDL / OpenAPI
      |
      v
Transport: Hertz generated routes + handlers
      |
      v
Application: use cases, orchestration, transactions
      |
      v
Domain: entities, policies, repository ports, capabilities
      |
      +--> Infra: PostgreSQL, Redis, locks, IDs
      |
      +--> Internal: HTTP providers, storage, notifications
```

API 层负责参数解析、身份解析、模型转换和统一响应。Application 层负责一个用户动作的完整业务流程。Domain 层拥有业务规则和端口。Infra/Internal 只负责技术实现。

## 2. Bounded Context

每个上下文至少有以下边界：

```text
idl/<context>/<context>.thrift
application/<context>/
domain/<context>/
internal/<context>/       # 仅当有外部系统
frontend/*/src/features/<context>/
```

上下文之间优先通过 application service 的明确方法组合，不通过 HTTP 回调自己，也不直接共享 repository 实现。

## 3. Composition Root

所有依赖在一个明确的 composition root 中装配：

```text
config -> database/cache/lock -> repositories/providers
       -> application services -> generated handlers -> HTTP server
```

禁止在 handler、repository 或 React 组件中创建全局数据库连接、Redis 客户端或第三方客户端。

## 4. Contract Flow

```text
Thrift IDL
  -> Hertz route/model/handler scaffolding
  -> OpenAPI document
  -> TypeScript schema for app/admin
```

修改 API 后必须检查：请求字段、响应字段、错误码、鉴权要求、前端生成类型和端到端行为。

## 5. Runtime topology

```text
app SPA       admin SPA
    \           /
       backend API
       /    |    \
     PG   Redis  external read-only sources/providers
```

生产环境可以将三个进程分别部署，但它们共享一个 API 契约和一个发布版本体系。数据库通常由平台提供，应用 chart 不应默认打包生产数据库。

