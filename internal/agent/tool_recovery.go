package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/eino/compose"
)

// Recover model dispatch mistakes through Eino's existing tool result channel.
// API and dispatch behavior: https://github.com/cloudwego/eino/blob/v0.9.13/compose/tool_node.go
// Middleware guide: https://www.cloudwego.io/docs/eino/core_modules/components/tools_node_guide/#middleware-mechanism
// Recovery consumes the existing agent round budget; it does not retry a tool.
func withToolRecovery(config compose.ToolsNodeConfig) compose.ToolsNodeConfig {
	config.UnknownToolsHandler = func(ctx context.Context, _, _ string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return `{"status":"unavailable_tool","nextStep":"该工具未注册，未执行任何操作。请使用本次提供的工具及其参数定义继续；无法完成时如实说明。"}`, nil
	}
	config.ToolCallMiddlewares = append(config.ToolCallMiddlewares, compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				output, err := next(ctx, input)
				if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return output, err
				}
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				// InferTool wraps argument decoding errors before calling the tool.
				// Match only that fixed prefix; do not hide actual execution errors.
				if strings.HasPrefix(err.Error(), "[LocalFunc] failed to unmarshal arguments") {
					return &compose.ToolOutput{Result: `{"status":"invalid_arguments","nextStep":"参数格式不符合工具定义，未执行查询。请按本次工具定义修正参数类型后继续，不要猜测查询结果。"}`}, nil
				}
				return output, err
			}
		},
	})
	return config
}
