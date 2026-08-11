# Domain

Domain 保存业务语言、领域规则、repository interface 和 capability/scope 定义。

具体数据库、Redis、HTTP 客户端不应被 domain 直接依赖。需要外部能力时，定义小而明确的端口，由 infra 或 internal 实现。

