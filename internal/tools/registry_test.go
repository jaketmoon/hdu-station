package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type testTool struct {
	name     string
	readOnly bool
}

func (tool testTool) Definition() Definition {
	return Definition{Name: tool.name, Description: "test", Parameters: json.RawMessage(`{"type":"object"}`)}
}

func (tool testTool) ReadOnly() bool { return tool.readOnly }

func (tool testTool) Call(context.Context, json.RawMessage) (Result, error) {
	return Result{Text: "ok"}, nil
}

func TestReadOnlyRegistrySortsDefinitionsAndCallsRegisteredTool(t *testing.T) {
	registry, err := NewReadOnlyRegistry(testTool{name: "z_tool", readOnly: true}, testTool{name: "a_tool", readOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	definitions := registry.Definitions()
	if len(definitions) != 2 || definitions[0].Name != "a_tool" || definitions[1].Name != "z_tool" {
		t.Fatalf("unexpected definitions: %#v", definitions)
	}
	result, err := registry.Call(context.Background(), "a_tool", nil)
	if err != nil || result.Text != "ok" {
		t.Fatalf("unexpected result: %#v, %v", result, err)
	}
}

func TestReadOnlyRegistryRejectsMutatingOrUnknownTools(t *testing.T) {
	if _, err := NewReadOnlyRegistry(testTool{name: "publish", readOnly: false}); err == nil || !strings.Contains(err.Error(), "not read-only") {
		t.Fatalf("unexpected mutating tool error: %v", err)
	}
	registry, err := NewReadOnlyRegistry()
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Call(context.Background(), "missing", nil)
	if !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("unexpected missing tool error: %v", err)
	}
}

func TestRegistryCallReadOnlyRejectsMutatingTools(t *testing.T) {
	registry, err := NewRegistry(
		testTool{name: "read", readOnly: true},
		testTool{name: "write", readOnly: false},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.CallReadOnly(context.Background(), "read", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.CallReadOnly(context.Background(), "write", nil); err == nil || !strings.Contains(err.Error(), "not read-only") {
		t.Fatalf("mutating tool was callable through read-only path: %v", err)
	}
}
