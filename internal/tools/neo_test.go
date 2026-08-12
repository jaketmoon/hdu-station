package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestNeoAcademicToolUsesAllowlistedReadOperation(t *testing.T) {
	tools := NewNeoAcademicTools("hduhelp-cli")
	var search *NeoAcademicTool
	for _, candidate := range tools {
		if candidate.Definition().Name == "hdu_academic_class_search" {
			search = candidate.(*NeoAcademicTool)
		}
	}
	if search == nil {
		t.Fatal("class search tool was not registered")
	}
	search.run = func(_ context.Context, binary string, arguments ...string) ([]byte, error) {
		if binary != "hduhelp-cli" {
			t.Errorf("binary = %q", binary)
		}
		want := []string{"academic", "class", "search", "--query", "数学", "--size", "10", "--json"}
		if strings.Join(arguments, "\x00") != strings.Join(want, "\x00") {
			t.Errorf("arguments = %#v, want %#v", arguments, want)
		}
		return []byte(`{"classes":[]}`), nil
	}
	arguments, err := json.Marshal(map[string]any{"query": "数学", "size": 10})
	if err != nil {
		t.Fatal(err)
	}
	result, err := search.Call(context.Background(), arguments)
	if err != nil || result.Text != `{"classes":[]}` {
		t.Fatalf("unexpected result: %#v, %v", result, err)
	}
}

func TestNeoAcademicToolRejectsUnknownArgumentsAndMissingCLI(t *testing.T) {
	tools := NewNeoAcademicTools("")
	selection := tools[1].(*NeoAcademicTool)
	selection.binary = "hduhelp-cli"
	_, err := selection.Call(context.Background(), json.RawMessage(`{"unknown":"value"}`))
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("unexpected unknown argument error: %v", err)
	}
	selection.binary = ""
	_, err = selection.Call(context.Background(), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("unexpected missing CLI error: %v", err)
	}
}
