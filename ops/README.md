# Operations

生产部署建议使用一个 chart 管理三个 workload：backend、app、admin。

- 数据库和 Redis 默认连接外部托管服务
- 静态配置和凭据通过 Secret/ConfigMap 注入
- 运营配置进入管理端和数据库，避免每次改运营参数都重新发布镜像
- API 使用稳定的 `/api-prefix`，SPA 使用 fallback 路由
- liveness、readiness、request id、结构化日志必须统一

