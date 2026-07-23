package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureAndReply is a chat-completions server that stashes the decoded request
// body and returns a caller-supplied canned reply (message + finish_reason).
func captureAndReply(t *testing.T, got *map[string]any, reply map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(got)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func toolReply(message map[string]any, finish string, in, out int64) map[string]any {
	return map[string]any{
		"choices": []map[string]any{{"message": message, "finish_reason": finish}},
		"usage":   map[string]any{"prompt_tokens": in, "completion_tokens": out},
	}
}

// TestOpenAICompatNativeRequestShape (provider-wire.md §2/§5a): native mode
// renders tools as [{type:function, function:{name,description,parameters}}],
// assistant turns as tool_calls with function.arguments JSON-encoded as a
// STRING, and tool results as role:"tool" messages — and never sends
// response_format alongside tools.
func TestOpenAICompatNativeRequestShape(t *testing.T) {
	var got map[string]any
	srv := captureAndReply(t, &got, toolReply(map[string]any{"content": "hi"}, "stop", 1, 1))
	o := newOpenAICompat(srv.URL, "m", "", "", ToolModeNative)

	req := Request{
		System: "sys",
		Tools: []ToolDecl{{Name: "drop", Description: "drop it",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"target":{"type":"string"}},"required":["target"],"additionalProperties":false}`)}},
		Turns: []Turn{
			{Role: RoleUser, Blocks: []Block{{Text: "go"}}},
			{Role: RoleAssistant, Blocks: []Block{{ToolUse: &ToolUseBlock{ID: "c1", Name: "drop", Args: json.RawMessage(`{"target":"axe"}`)}}}},
			{Role: RoleUser, Blocks: []Block{{ToolResult: &ToolResultBlock{ForID: "c1", Content: "ok"}}}},
		},
	}
	if _, err := o.call(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	tools, ok := got["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools missing/wrong: %v", got["tools"])
	}
	fn := tools[0].(map[string]any)
	if fn["type"] != "function" {
		t.Errorf("tool type = %v, want function", fn["type"])
	}
	f := fn["function"].(map[string]any)
	if f["name"] != "drop" || f["description"] != "drop it" {
		t.Errorf("function meta wrong: %v", f)
	}
	if params, ok := f["parameters"].(map[string]any); !ok || params["type"] != "object" || params["additionalProperties"] != false {
		t.Errorf("parameters schema not carried through: %v", f["parameters"])
	}

	msgs := got["messages"].([]any)
	// system, user, assistant(tool_calls), tool
	if len(msgs) != 4 {
		t.Fatalf("want 4 messages, got %d: %v", len(msgs), msgs)
	}
	asst := msgs[2].(map[string]any)
	tcs := asst["tool_calls"].([]any)
	tc := tcs[0].(map[string]any)
	if tc["id"] != "c1" || tc["type"] != "function" {
		t.Errorf("tool_call meta wrong: %v", tc)
	}
	tf := tc["function"].(map[string]any)
	// arguments MUST be a JSON-encoded string, not a nested object.
	argStr, ok := tf["arguments"].(string)
	if !ok {
		t.Fatalf("function.arguments must be a string, got %T: %v", tf["arguments"], tf["arguments"])
	}
	if argStr != `{"target":"axe"}` {
		t.Errorf("arguments = %q, want %q", argStr, `{"target":"axe"}`)
	}
	tool := msgs[3].(map[string]any)
	if tool["role"] != "tool" || tool["tool_call_id"] != "c1" || tool["content"] != "ok" {
		t.Errorf("tool result message wrong: %v", tool)
	}
}

// TestOpenAICompatNativeParse (provider-wire.md §2/§5b): the response's
// function.arguments string decodes back to a RawMessage, tool calls keep
// emission order, and finish_reason maps.
func TestOpenAICompatNativeParse(t *testing.T) {
	var got map[string]any
	msg := map[string]any{
		"content": "",
		"tool_calls": []map[string]any{
			{"id": "c1", "type": "function", "function": map[string]any{"name": "drop", "arguments": `{"target":"axe"}`}},
			{"id": "c2", "type": "function", "function": map[string]any{"name": "muse", "arguments": `{}`}},
		},
	}
	srv := captureAndReply(t, &got, toolReply(msg, "tool_calls", 7, 3))
	o := newOpenAICompat(srv.URL, "m", "", "", ToolModeNative)

	cr, err := o.call(context.Background(),
		Request{Tools: []ToolDecl{{Name: "drop", InputSchema: json.RawMessage(`{"type":"object"}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	if cr.stop != StopToolUse {
		t.Errorf("stop = %q, want tool_use", cr.stop)
	}
	if cr.inTok != 7 || cr.outTok != 3 {
		t.Errorf("tokens = (%d,%d), want (7,3)", cr.inTok, cr.outTok)
	}
	if len(cr.toolCalls) != 2 {
		t.Fatalf("want 2 calls, got %d", len(cr.toolCalls))
	}
	if cr.toolCalls[0].ID != "c1" || cr.toolCalls[0].Name != "drop" || string(cr.toolCalls[0].Args) != `{"target":"axe"}` {
		t.Errorf("call[0] = %+v", cr.toolCalls[0])
	}
	if cr.toolCalls[1].ID != "c2" || string(cr.toolCalls[1].Args) != `{}` {
		t.Errorf("call[1] = %+v", cr.toolCalls[1])
	}
}

// TestOpenAICompatFinishReasonMapping: every finish_reason maps to the right
// StopReason on a plain (no tool) reply.
func TestOpenAICompatFinishReasonMapping(t *testing.T) {
	cases := []struct {
		finish string
		want   StopReason
	}{
		{"stop", StopEndTurn},
		{"tool_calls", StopToolUse},
		{"length", StopMaxTokens},
		{"content_filter", StopOther},
		{"", StopOther},
	}
	for _, c := range cases {
		var got map[string]any
		srv := captureAndReply(t, &got, toolReply(map[string]any{"content": "x"}, c.finish, 1, 1))
		o := newOpenAICompat(srv.URL, "m", "", "", ToolModeNative)
		cr, err := o.call(context.Background(), Request{Prompt: "x"})
		if err != nil {
			t.Fatal(err)
		}
		if cr.stop != c.want {
			t.Errorf("finish_reason %q → %q, want %q", c.finish, cr.stop, c.want)
		}
	}
}

// TestOpenAICompatJSONRequestShape (provider-wire.md §3/§5): the fallback sends
// the envelope response_format, renders the tool catalog into the system
// prompt, echoes assistant turns as the envelope, and maps tool_result blocks
// to plain user text (result vs error) — with no role:"tool" messages.
func TestOpenAICompatJSONRequestShape(t *testing.T) {
	var got map[string]any
	srv := captureAndReply(t, &got, toolReply(map[string]any{"content": `{"tool":"none","say":"done"}`}, "stop", 1, 1))
	o := newOpenAICompat(srv.URL, "m", "", "", ToolModeJSON)

	req := Request{
		System: "sys",
		Tools: []ToolDecl{
			{Name: "drop", Description: "drop it", InputSchema: json.RawMessage(`{"type":"object","properties":{"target":{"type":"string"}}}`)},
			{Name: "muse", Description: "think", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		Turns: []Turn{
			{Role: RoleUser, Blocks: []Block{{Text: "go"}}},
			{Role: RoleAssistant, Blocks: []Block{{Text: "dropping"}, {ToolUse: &ToolUseBlock{ID: "env-1", Name: "drop", Args: json.RawMessage(`{"target":"axe"}`)}}}},
			{Role: RoleUser, Blocks: []Block{{ToolResult: &ToolResultBlock{ForID: "env-1", Content: "dropped"}}}},
			{Role: RoleUser, Blocks: []Block{{ToolResult: &ToolResultBlock{ForID: "env-1", Content: "no such target", IsError: true}}}},
		},
	}
	if _, err := o.call(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	// Envelope response_format present and well-formed.
	rf := got["response_format"].(map[string]any)
	js := rf["json_schema"].(map[string]any)
	if js["name"] != "tool_call" {
		t.Errorf("json_schema.name = %v, want tool_call", js["name"])
	}
	schema := js["schema"].(map[string]any)
	props := schema["properties"].(map[string]any)
	enum := props["tool"].(map[string]any)["enum"].([]any)
	if len(enum) != 3 || enum[0] != "drop" || enum[1] != "muse" || enum[2] != "none" {
		t.Errorf("tool enum = %v, want [drop muse none]", enum)
	}
	if props["say"].(map[string]any)["maxLength"].(float64) != 400 {
		t.Errorf("say maxLength = %v, want 400", props["say"])
	}
	if schema["additionalProperties"] != false {
		t.Errorf("envelope additionalProperties = %v, want false", schema["additionalProperties"])
	}

	msgs := got["messages"].([]any)
	// system, user(go), assistant(envelope), user(result), user(error)
	if len(msgs) != 5 {
		t.Fatalf("want 5 messages, got %d: %v", len(msgs), msgs)
	}
	sys := msgs[0].(map[string]any)
	if sys["role"] != "system" {
		t.Fatalf("first message not system: %v", sys)
	}
	sysText := sys["content"].(string)
	for _, want := range []string{"sys", "drop: drop it", "muse: think", `"tool"`} {
		if !strings.Contains(sysText, want) {
			t.Errorf("system prompt missing %q:\n%s", want, sysText)
		}
	}
	for _, m := range msgs {
		if m.(map[string]any)["role"] == "tool" {
			t.Errorf("json mode must not emit role:tool messages: %v", m)
		}
	}
	// tool_result → user text, result vs error labeled by tool name.
	if r := msgs[3].(map[string]any); r["role"] != "user" || r["content"] != "Tool result (drop): dropped" {
		t.Errorf("result mapping wrong: %v", r)
	}
	if e := msgs[4].(map[string]any); e["content"] != "Tool error (drop): no such target" {
		t.Errorf("error mapping wrong: %v", e)
	}
}

// TestOpenAICompatJSONParse (provider-wire.md §3): the envelope reply parses to
// one synthesized ToolCall (id env-<round>) when tool != "none", or to final
// text with StopEndTurn when tool == "none". The round ordinal derives from the
// count of assistant turns already in the transcript.
func TestOpenAICompatJSONParse(t *testing.T) {
	tools := []ToolDecl{{Name: "drop", InputSchema: json.RawMessage(`{"type":"object"}`)}}

	// tool != none, first round (no prior assistant turns) → env-1.
	var got map[string]any
	srv := captureAndReply(t, &got, toolReply(map[string]any{"content": `{"tool":"drop","args":{"target":"axe"},"say":"here goes"}`}, "stop", 4, 2))
	o := newOpenAICompat(srv.URL, "m", "", "", ToolModeJSON)
	cr, err := o.call(context.Background(), Request{Tools: tools, Prompt: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if cr.stop != StopToolUse {
		t.Errorf("stop = %q, want tool_use", cr.stop)
	}
	if len(cr.toolCalls) != 1 || cr.toolCalls[0].ID != "env-1" || cr.toolCalls[0].Name != "drop" ||
		string(cr.toolCalls[0].Args) != `{"target":"axe"}` {
		t.Fatalf("call = %+v", cr.toolCalls)
	}
	if cr.text != "here goes" || cr.inTok != 4 || cr.outTok != 2 {
		t.Errorf("cr = %+v", cr)
	}

	// A transcript with one prior assistant turn → next synthesized id is env-2.
	srv2 := captureAndReply(t, &got, toolReply(map[string]any{"content": `{"tool":"drop","args":{},"say":""}`}, "stop", 1, 1))
	o2 := newOpenAICompat(srv2.URL, "m", "", "", ToolModeJSON)
	cr2, err := o2.call(context.Background(), Request{
		Tools: tools,
		Turns: []Turn{
			{Role: RoleUser, Blocks: []Block{{Text: "go"}}},
			{Role: RoleAssistant, Blocks: []Block{{ToolUse: &ToolUseBlock{ID: "env-1", Name: "drop", Args: json.RawMessage(`{}`)}}}},
			{Role: RoleUser, Blocks: []Block{{ToolResult: &ToolResultBlock{ForID: "env-1", Content: "ok"}}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cr2.toolCalls) != 1 || cr2.toolCalls[0].ID != "env-2" {
		t.Errorf("second-round id = %+v, want env-2", cr2.toolCalls)
	}
	// empty args normalize to {}.
	if string(cr2.toolCalls[0].Args) != `{}` {
		t.Errorf("empty args = %q, want {}", cr2.toolCalls[0].Args)
	}

	// tool == none → final answer, no calls, StopEndTurn.
	srv3 := captureAndReply(t, &got, toolReply(map[string]any{"content": `{"tool":"none","say":"all set"}`}, "stop", 1, 1))
	o3 := newOpenAICompat(srv3.URL, "m", "", "", ToolModeJSON)
	cr3, err := o3.call(context.Background(), Request{Tools: tools, Prompt: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if cr3.stop != StopEndTurn || len(cr3.toolCalls) != 0 || cr3.text != "all set" {
		t.Errorf("done envelope: cr = %+v", cr3)
	}
}

// TestOpenAICompatJSONModeNoToolsFallsToNative: json tool_mode with no declared
// tools takes the native path (no envelope machinery), so a plain reply is
// unaffected by the knob.
func TestOpenAICompatJSONModeNoToolsFallsToNative(t *testing.T) {
	var got map[string]any
	srv := captureAndReply(t, &got, toolReply(map[string]any{"content": "plain"}, "stop", 1, 1))
	o := newOpenAICompat(srv.URL, "m", "", "", ToolModeJSON)
	cr, err := o.call(context.Background(), Request{Prompt: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["response_format"]; ok {
		t.Errorf("no-tools json-mode call must not send the envelope: %v", got["response_format"])
	}
	if cr.text != "plain" {
		t.Errorf("text = %q, want plain", cr.text)
	}
}
