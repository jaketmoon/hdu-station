# 收藏与模拟课表管理接入

2026-09-17。按用户最新要求移除 `internal/skills/course-favorites/SKILL.md`，新增：

- `course-collection-management`：收藏查询、排行、添加、删除、换班、整体替换、清空。
- `course-simulation-management`：模拟课表查询、模拟加入/退课、换班、撤销模拟操作、整份替换、重置。

两份 Skill 与推荐、开课核实和课表适配自由组合，无固定工作流。旧 `add_course_favorites` 不再注册给 Agent，改为 `manage_course_collection`；新增 `manage_course_simulation`。学校真实选退课保持不可调用。

## 授权与数据边界

Station 原有六项授权已包含 coursefavorite 和 coursesimulation 各自 read/write，无需额外 scope。本轮验证保存的授权确实拥有这四项权限，因此现有登录无需重做。设置文案与 AGENTS、architecture 已同步新能力。

收藏局部变更先读最新列表、保留其他项，新增目标必须来自本轮开课核实或已读收藏；删除先定位已读收藏。清空/整体替换要求与本轮读取快照相符，列表变化时停止。进程内串行处理，写后复查；上游无 CAS，其他客户端同时全量覆盖仍有竞争风险。

模拟方案先读当前学期及 revision，写前再次读取并校验版本，固定 PUT 携带 expectedRevision；按用户意图合并完整目标，保留未指定条目。只有真实课程可以模拟 DROP；模拟加入的删除和模拟退课的撤销均通过移除相应操作实现。新班必须属于本轮核实学期。要求无冲突时校验修改后整份方案的全部周次节次，未知时间不放行。409 或未知结果不自动重写。

课程详情只解码公开课程字段，不解析 classList；模拟响应只向模型传递必要课程和状态，不透传逐格 slots 或学生身份数据。固定 API 路由，不调用 CLI shell、不接收任意 URL 或学号。工具回执由宿主显示，模型中断时也保留已经取得的结果。

## 验证

- `make test`：Go 全部通过；前端43个测试通过。
- `make frontend-check`：由 test/build 依赖执行，类型检查和前端构建通过。
- `make build`：生成 `build/bin/HDU Station.app`。
- `go test -race ./internal/tools -run 'Test(Collection|Simulation|.*Favorite)' -count=1`：通过。
- 隔离 HTTP 测试覆盖收藏 CRUD/排行、换班保留其他收藏、清空前列表变化、模拟增删改查/重置、恢复真实课、跨学期、重复条目、时间冲突/未知、版本并发、响应丢失、读回失败和禁止自动重试。
- 模型错误/虚构成功测试：真实工具回执不被覆盖，模型失败时仍显示回执。
- 桌面1280×820、窄窗口390×844设置页 E2E 各1项通过，并查看截图确认管理权限说明完整可读。这两项使用测试 fixture。

## 真实接口与模型验收

`HDU_STATION_MANAGEMENT_READ_TEST=1 go test ./internal/tools -run '^TestLiveCourseManagementReads$' -v -count=1 -timeout=3m` 通过：实际收藏、排行和模拟课表均可读取，四项管理权限均存在。未发 POST/PUT。

新增 `internal/agent/testdata/management_questions.json`，4组真实模型题目：

| ID | 请求 | 结果 |
| --- | --- | --- |
| M01 | 只看收藏，不读课表 | 收藏列表与详情正确显示 |
| M02 | 只看模拟课表，区分真实/模拟 | 模拟课表工具读取并显示状态 |
| M03 | 组合查看收藏和模拟课表 | 两个工具均调用，分别展示 |
| M04 | 只查收藏排行 | 排行工具读取，未当作本人收藏 |

4组通过，无错误、空回答或写入进度；真实结果保存在应用数据根目录 `logs/management-qa-20260917/`。这轮只读真实账号；删除、替换和重置使用隔离数据测试，没有向用户账号提交修改收藏或模拟方案的写请求。GET 模拟课表可能按上游规则同步真实课表快照。
