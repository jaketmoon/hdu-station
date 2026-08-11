# Database Models

这里的 Go struct 是 gorm/gen 的输入。新增或修改模型后运行：

```bash
make gen-dal
```

业务查询进入 `domain/<context>/repository`，通过生成的字段表达式完成，禁止手写 SQL 字符串、`.Raw` 和 `.Exec`。

