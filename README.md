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

首次使用在助手设置填写模型地址与 API Key；QQ 复用本机腾讯频道 CLI 的登录。展开来源后可切换启用、扫码重新连接、清除登录凭证；缺少 QQ 连接组件时会在重连时准备。选课助手不执行选课、发帖等业务写操作。

赞哦需填写自己的 Token 和学校别名；小红书需先运行本机 `xiaohongshu-mcp`，再在设置中启用并扫码连接。两项默认关闭，详细步骤见 [搜索来源配置](docs/search-sources.md)。

在助手设置展开「HDU CLI 登录」，点击「网页授权」，在官方页面确认课程信息和本人课表的读取权限。已有登录如果仅含课程权限，点击「重新授权」补充课表权限。设置里的状态仍只检查本机登录，不会主动读取私人课表。

推荐后可追问“这些课本学期开哪些班？”或“结合我的课表，空闲时间能塞哪几门课？周五不要排课”。助手按教务默认学期核实开课；网上名称模糊时先查完整名称，再查核心词，按课程号整理候选并由模型匹配，可以保留多个近似名称。课表筛选按整学期周次、星期和节次计算，保留冲突与未确认状态，并给出一组彼此无冲突的候选班级。课程余量、选课资格、考试冲突和学分认定尚未核实，不执行真实选课；课程分类查询仍未接入。

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
