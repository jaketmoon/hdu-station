# 开发

需要 Go 1.25+、Node.js 20.19+ 和目标系统的 Wails 2 编译依赖。

```sh
make frontend-install
make test
make build
```

`frontend/src/api.ts` 是前端唯一的宿主边界。使用 `wails dev` 可运行完整桌面开发环境；单独 `npm run dev` 可查看界面，但真实问答只能由 Wails 提供，不会转到远端后端或返回演示数据。

`make e2e` 使用已安装的 Google Chrome，在测试文件里注入 Wails 替身，检查 1280×820 与 390×844。截图写入 `frontend/test-results/`。单测覆盖中文输入法、对话切换、流式停止、配置保留、SQLite 恢复、SSE 截断和工具作用域。

`make live-test` 是显式的真实联网验收；未设置 `HDU_STATION_LIVE_TEST=1` 时自动跳过。不要在日志、截图或测试失败信息中输出凭证。

源码结构：

- `app.go`：Wails 用例、单轮生命周期和取消。
- `internal/agent`：Eino 循环和 DeepSeek SSE 适配。
- `internal/tools`：固定只读 QQ 操作和现有完整性安装器。
- `internal/skills`：简短选课提示与唯一的频道范围数据。
- `internal/config` / `internal/storage`：私有 YAML 与本机对话历史。
- `frontend/src`：聊天界面、设置、Markdown 渲染和绑定契约。

没有通用 Agent、复杂证据账本、动态技能安装、Sandbox 或线上租户依赖。
