# Vibe Coding Full-Stack Template

这是一个可复制的前后端一体化项目架构母版，提取自 hduhelp-neo 的通用部分。

它适合需要以下能力的项目：

- 一个 Go 单体同时提供多个业务上下文的 HTTP API
- Thrift IDL 作为 API 契约源，生成 Hertz 路由、模型和 OpenAPI
- GORM + gorm/gen 提供类型安全的数据访问
- React 用户端和 React 管理端共享同一份 OpenAPI 类型
- PostgreSQL 作为主库，Redis 承载会话、缓存、锁和限流
- 第三方服务通过 internal 端口接入，并支持本地 fake
- 用 `AGENTS.md`、契约生成和固定验证命令约束 AI 编码

## 如何使用

1. 复制整个 `project-template/` 到新的 GitHub 仓库。
2. 全局替换 `YOUR_GO_MODULE`、`YOUR_PROJECT`、模块名、域名和部署配置。
3. 先定义业务上下文，再复制对应的 `idl/<context>`、`application/<context>`、`domain/<context>` 和前端 feature 目录。
4. 让 Agent 按 `docs/vibe-coding-workflow.md` 的纵向切片流程实现第一个真实功能。
5. 第一个功能完成后，再补充通用组件，不要一次性生成所有业务模块。

## 发布到自己的 GitHub

在仓库根目录执行：

```bash
cp -R project-template ../YOUR_PROJECT
cd ../YOUR_PROJECT
git init
git add .
git commit -m "chore: initialize full-stack architecture template"
git branch -M main
git remote add origin git@github.com:YOUR_ACCOUNT/YOUR_PROJECT.git
git push -u origin main
```

如果 GitHub 仓库已经创建，也可以直接把 `project-template/` 的内容复制到该仓库根目录后提交。当前模板不包含原仓库 Git 历史，也不会自动创建远程仓库。

## 目录

```text
.
├── idl/                    # API 契约：Thrift 是唯一事实源
├── api/                    # hz 生成的协议模型、路由和 handler
├── application/            # 用例编排和跨领域事务
├── domain/                 # 领域模型、仓储端口、权限能力
├── infra/                  # 数据库、缓存、锁、ID、日志等基础设施
├── internal/               # 第三方客户端和外部系统适配器
├── types/dbmodel/          # GORM 输入模型和生成查询
├── config/                 # 静态配置和环境加载
├── frontend/app/           # 用户端 React SPA
├── frontend/admin/         # 管理端 React SPA
├── docs/                   # 架构、需求和协作规则
├── ops/                    # Docker、Helm、Ingress、运行手册
└── scripts/                # 代码生成和本地检查脚本
```

## 核心请求链路

```text
Browser
  -> Vite proxy / Nginx
  -> /api-prefix/*
  -> Hertz global middleware
  -> generated route
  -> handler: protocol + auth adaptation
  -> application use case
  -> domain port implementation
  -> PostgreSQL / Redis / external service
  -> { data, code, msg }
```

## 生成链

```text
idl/**/*.thrift
  -> hz
  -> api/ + openapi.yaml
  -> openapi-typescript
  -> frontend/*/src/api/schema.d.ts

types/dbmodel/*.go
  -> gorm gen
  -> types/dbmodel/generated
```

生成物必须提交，契约修改必须同时更新后端和两个前端的类型文件。

## 重要边界

- `api` 只做 HTTP 协议适配，不放核心业务规则。
- `application` 只编排用例，不依赖 Hertz 细节。
- `domain` 依赖接口，不依赖具体数据库或第三方 SDK。
- `infra` 和 `internal` 实现 domain/application 需要的端口。
- 所有 SQL 使用生成的结构化查询，不允许散落字符串 SQL。
- 第三方客户端必须有真实实现、配置开关和确定性 fake；核心数据源可以明确声明为不可降级。
- 用户 token、运营者 token 和第三方授权 token 分开处理。

## 模板状态

这是架构和协作模板，不是已经可以直接编译的业务 starter。它刻意不包含具体业务 API、数据库表和前端依赖锁文件；复制后由 Agent 根据第一个真实功能补齐这些内容。

## 这不是业务代码模板

模板不包含具体学校、用户字段、第三方凭据、生产域名或业务上下文。复制后应先完成：

- 项目名称和 Go module
- API 前缀和前端环境变量
- 业务上下文列表
- 数据库模型和迁移策略
- 用户端、管理端的认证模型
- Docker/Helm 的环境配置
