package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type NeoAcademicTool struct {
	name        string
	description string
	command     string
	binary      string
	run         CommandRunner
	buildArgs   func(map[string]json.RawMessage) ([]string, error)
}

func NewNeoAcademicTools(binary string) []ReadOnlyTool {
	if binary == "" {
		if resolved, err := exec.LookPath("hduhelp-cli"); err == nil {
			binary = resolved
		}
	}
	return []ReadOnlyTool{
		&NeoAcademicTool{
			name: "hdu_academic_class_search", description: "Search available HDU teaching classes by keyword.", command: "academic class search", binary: binary, run: runCommand,
			buildArgs: func(input map[string]json.RawMessage) ([]string, error) {
				args := []string{"academic", "class", "search"}
				return appendNeoQueryArgs(args, input, []string{"query", "size", "from"})
			},
		},
		&NeoAcademicTool{
			name: "hdu_academic_course_selection", description: "Read the current user's HDU course selections.", command: "academic course-selection", binary: binary, run: runCommand,
			buildArgs: func(input map[string]json.RawMessage) ([]string, error) {
				args := []string{"academic", "course-selection"}
				return appendNeoQueryArgs(args, input, []string{"school_year", "semester"})
			},
		},
		&NeoAcademicTool{
			name: "hdu_academic_schedule", description: "Read the current user's HDU semester timetable.", command: "academic schedule", binary: binary, run: runCommand,
			buildArgs: func(input map[string]json.RawMessage) ([]string, error) {
				args := []string{"academic", "schedule"}
				return appendNeoQueryArgs(args, input, []string{"timestamp", "school_year", "semester", "week"})
			},
		},
		&NeoAcademicTool{
			name: "hdu_academic_schedule_now", description: "Read today's and tomorrow's HDU timetable.", command: "academic schedule now", binary: binary, run: runCommand,
			buildArgs: func(map[string]json.RawMessage) ([]string, error) {
				return []string{"academic", "schedule", "now"}, nil
			},
		},
	}
}

func (tool *NeoAcademicTool) Definition() Definition {
	return Definition{
		Name:        tool.name,
		Description: tool.description,
		Parameters:  neoParameters(tool.name),
	}
}

func (tool *NeoAcademicTool) ReadOnly() bool { return true }

func (tool *NeoAcademicTool) Call(ctx context.Context, arguments json.RawMessage) (Result, error) {
	if tool.binary == "" {
		return Result{}, errors.New("hduhelp-cli is not installed or not on PATH")
	}
	if tool.run == nil {
		return Result{}, errors.New("hduhelp-cli runner is not initialized")
	}
	input := map[string]json.RawMessage{}
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &input); err != nil {
			return Result{}, fmt.Errorf("decode hduhelp-cli arguments: %w", err)
		}
	}
	args, err := tool.buildArgs(input)
	if err != nil {
		return Result{}, err
	}
	args = append(args, "--json")
	output, err := tool.run(ctx, tool.binary, args...)
	if err != nil {
		return Result{}, fmt.Errorf("run hduhelp-cli %s: %w", tool.command, err)
	}
	if len(output) > maxTencentOutput {
		return Result{}, errors.New("hduhelp-cli response exceeds the 1 MiB limit")
	}
	return Result{Text: string(output)}, nil
}

func appendNeoQueryArgs(args []string, input map[string]json.RawMessage, allowed []string) ([]string, error) {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	for name := range input {
		if _, ok := allowedSet[name]; !ok {
			return nil, fmt.Errorf("hduhelp-cli argument %q is not allowed for this operation", name)
		}
	}
	for _, name := range allowed {
		raw, ok := input[name]
		if !ok {
			continue
		}
		value, err := rawString(raw)
		if err != nil {
			return nil, fmt.Errorf("hduhelp-cli argument %q must be a string or integer: %w", name, err)
		}
		if value == "" {
			continue
		}
		args = append(args, "--"+strings.ReplaceAll(name, "_", "-"), value)
	}
	return args, nil
}

func rawString(raw json.RawMessage) (string, error) {
	var stringValue string
	if json.Unmarshal(raw, &stringValue) == nil {
		return strings.TrimSpace(stringValue), nil
	}
	var integerValue int64
	if json.Unmarshal(raw, &integerValue) == nil {
		return strconv.FormatInt(integerValue, 10), nil
	}
	return "", errors.New("expected a string or integer")
}

func neoParameters(name string) json.RawMessage {
	switch name {
	case "hdu_academic_class_search":
		return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","maxLength":240},"size":{"type":"integer","minimum":1,"maximum":100},"from":{"type":"integer","minimum":0}},"additionalProperties":false}`)
	case "hdu_academic_course_selection":
		return json.RawMessage(`{"type":"object","properties":{"school_year":{"type":"string","maxLength":32},"semester":{"type":"string","maxLength":32}},"additionalProperties":false}`)
	case "hdu_academic_schedule":
		return json.RawMessage(`{"type":"object","properties":{"timestamp":{"type":"integer"},"school_year":{"type":"string","maxLength":32},"semester":{"type":"integer"},"week":{"type":"integer","minimum":1,"maximum":30}},"additionalProperties":false}`)
	default:
		return json.RawMessage(`{"type":"object","additionalProperties":false}`)
	}
}
