package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrToolNotFound = errors.New("tool not found")

type Definition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type Result struct {
	Text string `json:"text"`
}

type Tool interface {
	Definition() Definition
	ReadOnly() bool
	Call(context.Context, json.RawMessage) (Result, error)
}

// ReadOnlyTool is kept as a named contract for trusted external integrations.
// A Registry can also contain explicitly non-read-only tools such as the
// Station-owned Sandbox executor; those tools must be registered through
// NewRegistry, never NewReadOnlyRegistry.
type ReadOnlyTool = Tool

type Registry struct {
	tools        map[string]Tool
	readOnlyOnly bool
}

func NewReadOnlyRegistry(tools ...ReadOnlyTool) (*Registry, error) {
	registry := &Registry{tools: make(map[string]Tool, len(tools)), readOnlyOnly: true}
	for _, tool := range tools {
		if err := registry.Register(tool); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func NewRegistry(tools ...Tool) (*Registry, error) {
	registry := &Registry{tools: make(map[string]Tool, len(tools))}
	for _, tool := range tools {
		if err := registry.Register(tool); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *Registry) Register(tool Tool) error {
	if tool == nil {
		return errors.New("tool is required")
	}
	definition := tool.Definition()
	if strings.TrimSpace(definition.Name) == "" {
		return errors.New("tool name is required")
	}
	if r.readOnlyOnly && !tool.ReadOnly() {
		return fmt.Errorf("tool %q is not read-only", definition.Name)
	}
	if _, exists := r.tools[definition.Name]; exists {
		return fmt.Errorf("tool %q is already registered", definition.Name)
	}
	r.tools[definition.Name] = tool
	return nil
}

func (r *Registry) Definitions() []Definition {
	definitions := make([]Definition, 0, len(r.tools))
	for _, tool := range r.tools {
		definitions = append(definitions, tool.Definition())
	}
	sortDefinitions(definitions)
	return definitions
}

func (r *Registry) ReadOnlyDefinitions() []Definition {
	definitions := make([]Definition, 0, len(r.tools))
	for _, tool := range r.tools {
		if tool.ReadOnly() {
			definitions = append(definitions, tool.Definition())
		}
	}
	sortDefinitions(definitions)
	return definitions
}

func sortDefinitions(definitions []Definition) {
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
}

func (r *Registry) Call(ctx context.Context, name string, arguments json.RawMessage) (Result, error) {
	tool, ok := r.tools[name]
	if !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	return tool.Call(ctx, arguments)
}

// CallReadOnly is the narrow call path exposed to UI/debug bindings. It is
// intentionally checked at call time as well as registration time so a
// Registry that also contains sandbox_execute can never be used to bypass the
// read-only boundary.
func (r *Registry) CallReadOnly(ctx context.Context, name string, arguments json.RawMessage) (Result, error) {
	tool, ok := r.tools[name]
	if !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	if !tool.ReadOnly() {
		return Result{}, fmt.Errorf("tool %q is not read-only", name)
	}
	return tool.Call(ctx, arguments)
}
