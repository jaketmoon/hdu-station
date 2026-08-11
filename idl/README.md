# IDL

这里是 API 的唯一事实源。

建议结构：

```text
idl/
├── api.thrift          # 聚合所有上下文
├── base.thrift         # 公共 envelope、分页和错误结构
├── openapi.thrift      # HTTP/OpenAPI 注解
└── <context>/
    └── <context>.thrift
```

每个方法定义时同时考虑：HTTP method/path、query/header/body、鉴权类型、scope、响应结构和错误码。

