# Generated API Layer

`api/` 由 Hertz 根据 `idl/` 生成，通常包含：

- HTTP router
- request/response model
- handler scaffold
- middleware scaffold

生成代码和手写适配代码必须有清晰边界。handler 中允许保留协议转换、鉴权调用和响应映射，但核心业务规则应进入 application/domain。

