package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// NeoMCPEndpoint is deliberately fixed. A user-supplied endpoint would turn
	// a trusted campus tool into an arbitrary HTTP client, and it would make it
	// too easy to send the campus PAT to an unintended service.
	NeoMCPEndpoint    = "https://api.hduhelp.com/hduhelp-neo/mcp"
	neoMCPVersion     = "2026-07-28"
	neoMCPClientName  = "hdu-station"
	neoMCPVersionName = "0.1.0"
	maxNeoMCPOutput   = 1 << 20
)

// NewNeoMCPHTTPClient returns the only HTTP client the host-side Neo adapter
// should use. Neo carries the campus credential in Authorization, so a
// redirect is never safe to follow, even when the initial URL is fixed.
func NewNeoMCPHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("HDUHelp Neo MCP redirects are disabled")
		},
	}
}

type neoMCPClient struct {
	pat      string
	endpoint string
	client   *http.Client
	nextID   atomic.Uint64
}

type NeoMCPAcademicTool struct {
	client        *neoMCPClient
	name          string
	mcpName       string
	description   string
	requiredScope string
	parameter     json.RawMessage
	allowed       map[string]string
}

// NewNeoMCPAcademicTools creates the fixed academic read-only surface exposed
// by HDU Station. The PAT is retained only by this host-side client and is
// never included in a tool argument or passed to the Sandbox.
func NewNeoMCPAcademicTools(pat string, client *http.Client) []ReadOnlyTool {
	return newNeoMCPAcademicTools(pat, NeoMCPEndpoint, client)
}

func newNeoMCPAcademicTools(pat, endpoint string, client *http.Client) []ReadOnlyTool {
	if client == nil {
		client = NewNeoMCPHTTPClient()
	}
	mcp := &neoMCPClient{pat: strings.TrimSpace(pat), endpoint: endpoint, client: client}
	return []ReadOnlyTool{
		&NeoMCPAcademicTool{
			client: mcp, name: "hdu_academic_class_search", mcpName: "hduhelp.academic.class.search", requiredScope: "academic:course:read",
			description: "Search available HDU teaching classes by keyword through HDUHelp Neo.",
			parameter:   neoMCPParameters("hdu_academic_class_search"),
			allowed:     map[string]string{"query": "query", "size": "size", "from": "from"},
		},
		&NeoMCPAcademicTool{
			client: mcp, name: "hdu_academic_course_selection", mcpName: "hduhelp.academic.course-selection", requiredScope: "academic:studentselection:read",
			description: "Read the current user's HDU course selections through HDUHelp Neo.",
			parameter:   neoMCPParameters("hdu_academic_course_selection"),
			allowed:     map[string]string{"school_year": "schoolYear", "semester": "semester"},
		},
		&NeoMCPAcademicTool{
			client: mcp, name: "hdu_academic_schedule", mcpName: "hduhelp.academic.schedule", requiredScope: "academic:schedule:read",
			description: "Read the current user's HDU semester timetable through HDUHelp Neo.",
			parameter:   neoMCPParameters("hdu_academic_schedule"),
			allowed:     map[string]string{"timestamp": "timestamp", "school_year": "schoolYear", "semester": "semester", "week": "week"},
		},
		&NeoMCPAcademicTool{
			client: mcp, name: "hdu_academic_schedule_now", mcpName: "hduhelp.academic.schedule.now", requiredScope: "academic:schedule:read",
			description: "Read today's and tomorrow's HDU timetable through HDUHelp Neo.",
			parameter:   neoMCPParameters("hdu_academic_schedule_now"),
			allowed:     map[string]string{},
		},
	}
}

func (tool *NeoMCPAcademicTool) Definition() Definition {
	return Definition{Name: tool.name, Description: tool.description, Parameters: tool.parameter}
}

func (tool *NeoMCPAcademicTool) ReadOnly() bool { return true }

func (tool *NeoMCPAcademicTool) Call(ctx context.Context, arguments json.RawMessage) (Result, error) {
	if tool == nil || tool.client == nil {
		return Result{}, errors.New("HDUHelp Neo MCP client is not initialized")
	}
	if tool.client.pat == "" {
		return Result{}, errors.New("HDU campus key is not configured")
	}
	if err := validateNeoMCPEndpoint(tool.client.endpoint); err != nil {
		return Result{}, err
	}
	input, err := decodeObjectArguments(arguments)
	if err != nil {
		return Result{}, fmt.Errorf("decode HDUHelp Neo arguments: %w", err)
	}
	mapped, err := mapNeoMCPArguments(input, tool.allowed, tool.name)
	if err != nil {
		return Result{}, err
	}
	wiredArguments := mapped
	if len(tool.allowed) > 0 {
		wiredArguments, err = wrapNeoMCPArguments(mapped)
		if err != nil {
			return Result{}, err
		}
	}
	return tool.client.call(ctx, tool.mcpName, wiredArguments, tool.requiredScope)
}

// wrapNeoMCPArguments follows the generated Neo MCP input schema. Parameters
// are grouped by their HTTP location; all current academic tools expose their
// parameters in the query group. Keeping this conversion at the wire boundary
// lets the Agent-facing schema remain idiomatic snake_case JSON.
func wrapNeoMCPArguments(query map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	if query == nil {
		query = map[string]json.RawMessage{}
	}
	rawQuery, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("encode HDUHelp Neo query arguments: %w", err)
	}
	return map[string]json.RawMessage{"query": rawQuery}, nil
}

func (client *neoMCPClient) call(ctx context.Context, name string, arguments map[string]json.RawMessage, requiredScope string) (Result, error) {
	id := client.nextID.Add(1)
	body := neoMCPRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/call",
		Params: neoMCPCallParams{
			Name:      name,
			Arguments: arguments,
			Meta: map[string]any{
				"io.modelcontextprotocol/protocolVersion": neoMCPVersion,
				"io.modelcontextprotocol/clientInfo": map[string]string{
					"name": neoMCPClientName, "version": neoMCPVersionName,
				},
				"io.modelcontextprotocol/clientCapabilities": map[string]any{},
			},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return Result{}, fmt.Errorf("encode HDUHelp Neo MCP request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(data))
	if err != nil {
		return Result{}, fmt.Errorf("create HDUHelp Neo MCP request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.pat)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", neoMCPVersion)
	request.Header.Set("Mcp-Method", "tools/call")
	request.Header.Set("Mcp-Name", name)
	response, err := client.client.Do(request)
	if err != nil {
		return Result{}, fmt.Errorf("call HDUHelp Neo MCP: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxNeoMCPOutput+1))
	if err != nil {
		return Result{}, fmt.Errorf("read HDUHelp Neo MCP response: %w", err)
	}
	if len(responseBody) > maxNeoMCPOutput {
		return Result{}, errors.New("HDUHelp Neo MCP response exceeds the 1 MiB limit")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Result{}, fmt.Errorf("HDUHelp Neo MCP returned HTTP %d", response.StatusCode)
	}
	var envelope neoMCPResponse
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return Result{}, fmt.Errorf("decode HDUHelp Neo MCP response: %w", err)
	}
	if envelope.Error != nil {
		return Result{}, classifyNeoMCPFailure(envelope.Error.Message, requiredScope)
	}
	if envelope.Result.IsError {
		return Result{}, classifyNeoMCPFailure(envelope.Result.text(), requiredScope)
	}
	text := envelope.Result.text()
	if text == "" && len(envelope.Result.StructuredContent) > 0 {
		text = string(envelope.Result.StructuredContent)
	}
	if text == "" {
		return Result{}, errors.New("HDUHelp Neo MCP returned no tool content")
	}
	return Result{Text: text}, nil
}

// classifyNeoMCPFailure deliberately does not forward MCP error text. The
// upstream service is outside Station's trust boundary and an auth failure
// may contain a token fragment or other connector-owned data. The Agent only
// needs a safe category to decide whether to ask for a new PAT, request a
// missing scope, or report a transient integration failure.
func classifyNeoMCPFailure(message, requiredScope string) error {
	message, code, missingScope := extractNeoMCPErrorSignals(message)
	lower := strings.ToLower(strings.TrimSpace(message))
	code = strings.ToLower(strings.TrimSpace(code))
	switch {
	case code == "insufficient_scope" || code == "scope_insufficient" || code == "unauthenticated":
		if code != "unauthenticated" && requiredScope != "" {
			return fmt.Errorf("HDUHelp Neo authorization is missing the required scope: %s", requiredScope)
		}
		if code == "unauthenticated" {
			return errors.New("HDUHelp Neo credential was rejected")
		}
		return errors.New("HDUHelp Neo authorization is missing the required scope")
	case missingScope:
		if requiredScope != "" {
			return fmt.Errorf("HDUHelp Neo authorization is missing the required scope: %s", requiredScope)
		}
		return errors.New("HDUHelp Neo authorization is missing the required scope")
	case strings.Contains(lower, "insufficient_scope"),
		strings.Contains(lower, "scope_insufficient"),
		strings.Contains(lower, "missing_scopes"),
		strings.Contains(lower, "710000107"),
		strings.Contains(message, "权限不足"),
		strings.Contains(message, "缺少 scope"):
		if requiredScope != "" {
			return fmt.Errorf("HDUHelp Neo authorization is missing the required scope: %s", requiredScope)
		}
		return errors.New("HDUHelp Neo authorization is missing the required scope")
	case strings.Contains(lower, "invalid token"),
		strings.Contains(lower, "invalid ai token"),
		strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "forbidden"),
		strings.Contains(lower, "authentication"):
		return errors.New("HDUHelp Neo credential was rejected")
	default:
		return errors.New("HDUHelp Neo academic tool failed")
	}
}

// extractNeoMCPErrorSignals understands the error envelope emitted by the
// current Neo MCP bridge. It deliberately returns only bounded classification
// signals; the upstream message and details are never forwarded to the Agent.
func extractNeoMCPErrorSignals(value string) (message, code string, missingScope bool) {
	message = value
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details struct {
				MissingScope  string          `json:"missing_scope"`
				MissingScopes []string        `json:"missing_scopes"`
				Body          json.RawMessage `json:"body"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(value), &envelope); err != nil || envelope.Error.Code == "" {
		return message, "", false
	}
	code = envelope.Error.Code
	message = envelope.Error.Message
	missingScope = strings.TrimSpace(envelope.Error.Details.MissingScope) != "" || len(envelope.Error.Details.MissingScopes) > 0
	if len(envelope.Error.Details.Body) > 0 {
		var body struct {
			Code    json.RawMessage `json:"code"`
			Message string          `json:"message"`
			Msg     string          `json:"msg"`
		}
		if json.Unmarshal(envelope.Error.Details.Body, &body) == nil {
			if len(body.Code) > 0 {
				var numeric int64
				if json.Unmarshal(body.Code, &numeric) == nil && numeric == 710000107 {
					missingScope = true
				}
				var textual string
				if json.Unmarshal(body.Code, &textual) == nil && strings.Contains(strings.ToLower(textual), "scope") {
					missingScope = true
				}
			}
			bodyMessage := body.Message
			if bodyMessage == "" {
				bodyMessage = body.Msg
			}
			if strings.Contains(strings.ToLower(bodyMessage), "scope") || strings.Contains(bodyMessage, "权限") {
				missingScope = true
			}
		}
	}
	return message, code, missingScope
}

type neoMCPRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      uint64           `json:"id"`
	Method  string           `json:"method"`
	Params  neoMCPCallParams `json:"params"`
}

type neoMCPCallParams struct {
	Name      string                     `json:"name"`
	Arguments map[string]json.RawMessage `json:"arguments,omitempty"`
	Meta      map[string]any             `json:"_meta"`
}

type neoMCPResponse struct {
	Error  *neoMCPError     `json:"error,omitempty"`
	Result neoMCPCallResult `json:"result"`
}

type neoMCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type neoMCPCallResult struct {
	IsError           bool            `json:"isError"`
	Content           []neoMCPContent `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent"`
}

type neoMCPContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (result neoMCPCallResult) text() string {
	var parts []string
	for _, content := range result.Content {
		if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
			parts = append(parts, content.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func validateNeoMCPEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "api.hduhelp.com" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("HDUHelp Neo MCP endpoint is invalid")
	}
	return nil
}

func decodeObjectArguments(arguments json.RawMessage) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(arguments)) == 0 || string(bytes.TrimSpace(arguments)) == "null" {
		return map[string]json.RawMessage{}, nil
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &input); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, errors.New("arguments must be a JSON object")
	}
	return input, nil
}

func mapNeoMCPArguments(input map[string]json.RawMessage, allowed map[string]string, toolName string) (map[string]json.RawMessage, error) {
	mapped := make(map[string]json.RawMessage, len(input))
	for key, value := range input {
		mcpKey, ok := allowed[key]
		if !ok {
			return nil, fmt.Errorf("HDUHelp Neo argument %q is not allowed for this operation", key)
		}
		if !json.Valid(value) {
			return nil, fmt.Errorf("HDUHelp Neo argument %q is invalid JSON", key)
		}
		if err := validateNeoMCPArgument(toolName, key, value); err != nil {
			return nil, err
		}
		mapped[mcpKey] = value
	}
	return mapped, nil
}

func validateNeoMCPArgument(toolName, name string, raw json.RawMessage) error {
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return fmt.Errorf("HDUHelp Neo argument %q must not be null", name)
	}
	if name == "query" || name == "school_year" {
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return fmt.Errorf("HDUHelp Neo argument %q must be a string", name)
		}
		limit := 32
		if name == "query" {
			limit = 240
		}
		if len([]rune(value)) > limit {
			return fmt.Errorf("HDUHelp Neo argument %q exceeds the %d character limit", name, limit)
		}
		return nil
	}
	if name == "semester" && toolName == "hdu_academic_course_selection" {
		// Course-selection uses a string semester; schedule uses an integer.
		var stringValue string
		if json.Unmarshal(trimmed, &stringValue) == nil {
			if len([]rune(stringValue)) > 32 {
				return errors.New("HDUHelp Neo argument semester exceeds the 32 character limit")
			}
			return nil
		}
	}
	var integer int64
	if err := json.Unmarshal(trimmed, &integer); err != nil {
		return fmt.Errorf("HDUHelp Neo argument %q must be an integer", name)
	}
	switch name {
	case "size":
		if integer < 1 || integer > 100 {
			return errors.New("HDUHelp Neo argument size must be between 1 and 100")
		}
	case "from":
		if integer < 0 {
			return errors.New("HDUHelp Neo argument from must be non-negative")
		}
	case "week":
		if integer < 1 || integer > 30 {
			return errors.New("HDUHelp Neo argument week must be between 1 and 30")
		}
	}
	return nil
}

func neoMCPParameters(name string) json.RawMessage {
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
