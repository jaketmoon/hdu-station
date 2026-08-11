# Configuration

配置分为两类：

- 启动配置：数据库、Redis、外部凭据、监听地址、日志和部署参数
- 运营配置：业务开关、任务配置、租户或站点设置，优先进入管理端和数据库

禁止把真实凭据提交到 Git。提供 `config.example.yaml` 或 `.env.example`，并在启动时校验生产环境的必填项。

