# Commands

推荐保留以下入口：

```text
cmd/<project>/       # HTTP API 进程
cmd/<project>-cli/   # 可选：开发、运维或用户 CLI
```

如果定时任务和 API 共用同一进程，必须使用 Redis 分布式锁；如果拆成 worker，仍需复用同一套 application service 和配置约定。

