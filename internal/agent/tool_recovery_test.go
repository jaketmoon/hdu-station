package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func TestToolRecoveryAllowsValidCallsAfterModelMistakes(t *testing.T) {
	ctx := context.Background()
	calls := 0
	query, err := utils.InferTool("query", "test", func(_ context.Context, in struct {
		Names []string `json:"names"`
	}) (string, error) {
		calls++
		return strings.Join(in.Names, "、"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	config := withToolRecovery(compose.ToolsNodeConfig{
		Tools: []tool.BaseTool{query}, ExecuteSequentially: true,
	})
	node, err := compose.NewToolNode(ctx, &config)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, arguments, want string
		wantCalls             int
	}{
		{"private-tool-sentinel", `{"token":"private-argument-sentinel"}`, "unavailable_tool", 0},
		{"query", `{"names":"private-argument-sentinel"}`, "invalid_arguments", 0},
		{"query", `{"names":["戏曲鉴赏"]}`, "戏曲鉴赏", 1},
	} {
		message := schema.AssistantMessage("", []schema.ToolCall{{ID: "test-call", Type: "function", Function: schema.FunctionCall{Name: tc.name, Arguments: tc.arguments}}})
		results, err := node.Invoke(ctx, message)
		if err != nil || len(results) != 1 {
			t.Fatalf("tool result missing: %v", err)
		}
		if !strings.Contains(results[0].Content, tc.want) || strings.Contains(results[0].Content, "private-") || results[0].ToolCallID != "test-call" || calls != tc.wantCalls {
			t.Fatal("recovery lost result association, executed invalid input, or exposed raw details")
		}
	}
}

func TestToolRecoveryPreservesCancellationAndExecutionErrors(t *testing.T) {
	config := withToolRecovery(compose.ToolsNodeConfig{})
	middleware := config.ToolCallMiddlewares[0].Invokable
	for _, expected := range []error{context.Canceled, context.DeadlineExceeded, errors.New("real service failure"), errors.New("[LocalFunc] failed to invoke tool: failed to unmarshal arguments")} {
		next := middleware(func(context.Context, *compose.ToolInput) (*compose.ToolOutput, error) { return nil, expected })
		_, err := next(context.Background(), &compose.ToolInput{})
		if !errors.Is(err, expected) {
			t.Fatal("recovery hid a cancellation or execution failure")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := config.UnknownToolsHandler(ctx, "unknown", "{}"); !errors.Is(err, context.Canceled) {
		t.Fatal("unknown tool recovery ignored cancellation")
	}
	called := false
	_, err := middleware(func(context.Context, *compose.ToolInput) (*compose.ToolOutput, error) {
		called = true
		return nil, nil
	})(ctx, &compose.ToolInput{})
	if !errors.Is(err, context.Canceled) || called {
		t.Fatal("cancelled query executed")
	}
}
