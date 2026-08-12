package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type neoRoundTripFunc func(*http.Request) (*http.Response, error)

func (f neoRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestNeoMCPAcademicToolUsesModernReadOnlyCallAndMapsArguments(t *testing.T) {
	client := &http.Client{Transport: neoRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != NeoMCPEndpoint {
			t.Errorf("endpoint = %q", request.URL.String())
		}
		if got := request.Header.Get("Authorization"); got != "Bearer campus-pat" {
			t.Errorf("authorization = %q", got)
		}
		for header, want := range map[string]string{
			"MCP-Protocol-Version": neoMCPVersion,
			"Mcp-Method":           "tools/call",
			"Mcp-Name":             "hduhelp.academic.schedule",
		} {
			if got := request.Header.Get(header); got != want {
				t.Errorf("%s = %q, want %q", header, got, want)
			}
		}
		var envelope struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  struct {
				Name      string                     `json:"name"`
				Arguments map[string]json.RawMessage `json:"arguments"`
				Meta      map[string]any             `json:"_meta"`
			} `json:"params"`
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if envelope.JSONRPC != "2.0" || envelope.Method != "tools/call" || envelope.Params.Name != "hduhelp.academic.schedule" {
			t.Errorf("unexpected MCP envelope: %s", body)
		}
		var query map[string]json.RawMessage
		if err := json.Unmarshal(envelope.Params.Arguments["query"], &query); err != nil {
			t.Fatalf("decode query arguments: %v", err)
		}
		if got := string(query["schoolYear"]); got != `"2025-2026"` {
			t.Errorf("schoolYear = %s", got)
		}
		if _, exists := query["school_year"]; exists {
			t.Error("snake_case school_year leaked into MCP arguments")
		}
		if got := string(query["week"]); got != `2` {
			t.Errorf("week = %s", got)
		}
		if got := envelope.Params.Meta["io.modelcontextprotocol/protocolVersion"]; got != neoMCPVersion {
			t.Errorf("protocol metadata = %#v", got)
		}
		return neoJSONResponse(http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"status_code\":200,\"data\":{\"items\":[]}}"}],"structuredContent":{"status_code":200}}}`), nil
	})}

	tools := NewNeoMCPAcademicTools("campus-pat", client)
	var schedule *NeoMCPAcademicTool
	for _, candidate := range tools {
		if candidate.Definition().Name == "hdu_academic_schedule" {
			schedule = candidate.(*NeoMCPAcademicTool)
		}
	}
	if schedule == nil {
		t.Fatal("schedule MCP tool was not registered")
	}
	result, err := schedule.Call(context.Background(), json.RawMessage(`{"school_year":"2025-2026","week":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Text, `"status_code":200`) {
		t.Fatalf("unexpected result: %s", result.Text)
	}
}

func TestWrapNeoMCPArgumentsUsesQueryLocationGroup(t *testing.T) {
	wrapped, err := wrapNeoMCPArguments(map[string]json.RawMessage{
		"query": json.RawMessage(`"计算机"`),
		"size":  json.RawMessage(`1`),
		"from":  json.RawMessage(`0`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := wrapped["size"]; ok {
		t.Fatal("size escaped the query location group")
	}
	var query map[string]json.RawMessage
	if err := json.Unmarshal(wrapped["query"], &query); err != nil {
		t.Fatal(err)
	}
	if string(query["query"]) != `"计算机"` || string(query["size"]) != `1` || string(query["from"]) != `0` {
		t.Fatalf("unexpected query group: %s", wrapped["query"])
	}
}

func TestNeoMCPAcademicScheduleNowOmitsAbsentLocationGroups(t *testing.T) {
	client := &http.Client{Transport: neoRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		var envelope struct {
			Params struct {
				Arguments map[string]json.RawMessage `json:"arguments"`
			} `json:"params"`
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Fatal(err)
		}
		if len(envelope.Params.Arguments) != 0 {
			t.Fatalf("parameterless tool sent location groups: %s", body)
		}
		return neoJSONResponse(http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"ok"}]}}`), nil
	})}
	tools := NewNeoMCPAcademicTools("campus-pat", client)
	var now *NeoMCPAcademicTool
	for _, candidate := range tools {
		if candidate.Definition().Name == "hdu_academic_schedule_now" {
			now = candidate.(*NeoMCPAcademicTool)
		}
	}
	if now == nil {
		t.Fatal("schedule-now MCP tool was not registered")
	}
	if _, err := now.Call(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
}

func TestNeoMCPAcademicToolOnlyExposesFixedAcademicSurface(t *testing.T) {
	client := &http.Client{Transport: neoRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return neoJSONResponse(http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"ok"}]}}`), nil
	})}
	tools := NewNeoMCPAcademicTools("campus-pat", client)
	if len(tools) != 4 {
		t.Fatalf("tool count = %d, want 4", len(tools))
	}
	for _, tool := range tools {
		if !tool.ReadOnly() || !strings.HasPrefix(tool.Definition().Name, "hdu_academic_") {
			t.Fatalf("unexpected tool: %#v", tool.Definition())
		}
	}
	_, err := tools[0].Call(context.Background(), json.RawMessage(`{"unsupported":true}`))
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("unexpected unknown argument error: %v", err)
	}
	_, err = tools[0].Call(context.Background(), json.RawMessage(`{"size":0}`))
	if err == nil || !strings.Contains(err.Error(), "between 1 and 100") {
		t.Fatalf("unexpected bounded argument error: %v", err)
	}
}

func TestNeoMCPAcademicToolRejectsMissingPATAndMCPErrors(t *testing.T) {
	tools := NewNeoMCPAcademicTools("", nil)
	_, err := tools[0].Call(context.Background(), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "campus key") {
		t.Fatalf("unexpected missing PAT error: %v", err)
	}

	client := &http.Client{Transport: neoRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return neoJSONResponse(http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"isError":true,"content":[{"type":"text","text":"insufficient_scope"}]}}`), nil
	})}
	tool := NewNeoMCPAcademicTools("campus-pat", client)[0]
	_, err = tool.Call(context.Background(), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "missing the required scope: academic:course:read") {
		t.Fatalf("unexpected MCP tool error: %v", err)
	}
}

func TestNeoMCPAcademicToolClassifiesBridgeErrorEnvelopeWithoutLeakingDetails(t *testing.T) {
	responses := []struct {
		name string
		body string
		want string
	}{
		{
			name: "scope",
			body: `{"jsonrpc":"2.0","id":1,"result":{"isError":true,"content":[{"type":"text","text":"{\"error\":{\"code\":\"insufficient_scope\",\"message\":\"hidden\",\"details\":{\"missing_scopes\":[\"academic:course:read\"]}}}"}]}}`,
			want: "missing the required scope: academic:course:read",
		},
		{
			name: "nested-business-scope",
			body: `{"jsonrpc":"2.0","id":1,"result":{"isError":true,"content":[{"type":"text","text":"{\"error\":{\"code\":\"service_error\",\"message\":\"hidden\",\"details\":{\"body\":{\"code\":710000107,\"msg\":\"缺少 scope\"}}}}"}]}}`,
			want: "missing the required scope: academic:course:read",
		},
		{
			name: "unauthenticated",
			body: `{"jsonrpc":"2.0","id":1,"result":{"isError":true,"content":[{"type":"text","text":"{\"error\":{\"code\":\"unauthenticated\",\"message\":\"secret credential detail\"}}"}]}}`,
			want: "credential was rejected",
		},
	}
	for _, test := range responses {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: neoRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				return neoJSONResponse(http.StatusOK, test.body), nil
			})}
			tool := NewNeoMCPAcademicTools("campus-pat", client)[0]
			_, err := tool.Call(context.Background(), json.RawMessage(`{}`))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unexpected safe classification: %v", err)
			}
			if strings.Contains(err.Error(), "hidden") || strings.Contains(err.Error(), "secret credential detail") || strings.Contains(err.Error(), "缺少 scope") {
				t.Fatal("bridge error details escaped Neo adapter")
			}
		})
	}
}

func TestNeoMCPAcademicToolRedactsUpstreamCredentialErrors(t *testing.T) {
	const secret = "hduhelp_pat_secret-that-must-not-escape"
	responses := []string{
		`{"jsonrpc":"2.0","id":1,"result":{"isError":true,"content":[{"type":"text","text":"invalid ai token=` + secret + `"}]}}`,
		`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"invalid token ` + secret + `"}}`,
	}
	for index, body := range responses {
		t.Run(string(rune('a'+index)), func(t *testing.T) {
			client := &http.Client{Transport: neoRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				return neoJSONResponse(http.StatusOK, body), nil
			})}
			tool := NewNeoMCPAcademicTools("campus-pat", client)[0]
			_, err := tool.Call(context.Background(), json.RawMessage(`{}`))
			if err == nil || !strings.Contains(err.Error(), "credential was rejected") {
				t.Fatalf("unexpected redacted error: %v", err)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatal("upstream credential text escaped Neo adapter")
			}
		})
	}
}

func TestNeoMCPHTTPClientDoesNotFollowRedirectsWithAuthorization(t *testing.T) {
	redirected := false
	client := NewNeoMCPHTTPClient()
	client.Transport = neoRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/other" {
			redirected = true
		}
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"https://api.hduhelp.com/other"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})
	request, err := http.NewRequest(http.MethodPost, NeoMCPEndpoint, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer campus-pat")
	_, err = client.Do(request)
	if err == nil || !strings.Contains(err.Error(), "redirects are disabled") {
		t.Fatalf("redirect was not rejected: %v", err)
	}
	if redirected {
		t.Fatal("Neo HTTP client followed a redirect")
	}
}

func neoJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
