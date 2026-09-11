# HDU Station · 选课助手

打开就能问选课问题：通识选修有什么水课、哪门课作业少、老师给分如何。助手读取杭电 QQ 频道讨论，也可启用赞哦校园集市和小红书，使用 DeepSeek V4.1 Flash 给出自然回答，并附原帖链接或小程序帖子位置。

Go + Wails 2 · React + TypeScript · Eino · SQLite / YAML

## 运行

```sh
make frontend-install
make build
open "build/bin/HDU Station.app"
```

macOS 构建生成可直接打开的本地签名应用。它尚未做 Developer ID 公证，不作为公开发行包。Windows/Linux 的桌面构建需要 Wails 对应平台依赖。

首次使用在助手设置填写模型地址与 API Key；QQ 复用本机腾讯频道 CLI 的登录。如未安装连接组件，可在设置中安装。选课助手不执行选课、发帖或账号写操作。

赞哦需填写自己的 Token 和学校别名；小红书需先运行并登录本机 `xiaohongshu-mcp`，再在设置中启用。两项默认关闭，详细步骤见 [搜索来源配置](docs/search-sources.md)。

## 从原项目迁移连接

```sh
go run ./cmd/import-config -from /path/to/old-project/.env
```

仅迁移连接配置，模型改为 V4.1 Flash。已有新版配置不会覆盖。凭证保存到新版数据根目录，产品不会读取或打包 `.env`。

## 验证

```sh
make test
make frontend-check
make build
make e2e
make live-test
```

`make live-test` 会使用已配置模型和 QQ 身份，产生实际模型调用费用。三个问题覆盖轻松通识课、给分与考核、人文经典选修；结果位于本机数据目录的 `logs/acceptance.md`。

## 本机数据

macOS：`~/Library/Application Support/HDU Station Course`。关闭应用后删除此目录可清除配置、历史、连接组件及测试报告；旧版 `HDU Station` 数据和 QQ 自身的登录不受影响。

实现边界见 [架构](docs/architecture.md)，开发方式见 [开发说明](docs/development.md)。
