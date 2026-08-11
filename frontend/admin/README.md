# Admin App

管理端是独立 React + Vite SPA。

- 使用独立的运营者认证状态
- 使用 `openapi-fetch` 或项目统一的 typed client
- 业务错误通过统一的 `{ code, msg, data }` envelope 处理
- 管理操作必须有明确的权限、确认和失败反馈
- 不复用用户端 token store 或请求封装

