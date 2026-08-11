# Application

Application 是用例层。一个公开动作通常对应一个清晰的方法，例如：

```go
func (s *Service) CreateThing(ctx context.Context, actor Actor, input CreateInput) (*Thing, error)
```

这里编排 repository、领域规则、事务和外部端口；不要引用 Hertz request context、HTTP response 或 React 类型。

