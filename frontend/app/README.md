# User App

用户端是 React + Vite SPA。

- 请求统一经过 `src/services/neoClient.ts`
- API 类型来自 `src/api/schema.d.ts`
- 页面不直接拼接 token 或重复实现 refresh
- 每个页面覆盖 loading、empty、error、success 和未授权状态
- 真实浏览器验证移动端和桌面端布局

