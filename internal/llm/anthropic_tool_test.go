package llm

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// TestAnthropicBuildParamsGolden (TASK-52, provider-wire.md §1/§5a): a Request
// carrying Tools and a multi-turn transcript marshals to the exact native
// Messages shape — tools as {name, description, input_schema} (schema keywords
// including additionalProperties preserved), assistant tool_use echoes, user
// tool_result outcomes, and the ephemeral-cached system prompt.
func TestAnthropicBuildParamsGolden(t *testing.T) {
	a := &anthropicCaller{model: "claude-test"}
	req := Request{
		System:    "you are a villager",
		MaxTokens: 512,
		Tools: []ToolDecl{
			{Name: "drop", Description: "drop an item", InputSchema: json.RawMessage(`{"type":"object","properties":{"target":{"type":"string"}},"required":["target"],"additionalProperties":false}`)},
			{Name: "muse", Description: "have a thought", InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)},
		},
		Turns: []Turn{
			{Role: RoleUser, Blocks: []Block{{Text: "what now?"}}},
			{Role: RoleAssistant, Blocks: []Block{
				{Text: "let me drop it"},
				{ToolUse: &ToolUseBlock{ID: "tu_1", Name: "drop", Args: json.RawMessage(`{"target":"axe"}`)}},
			}},
			{Role: RoleUser, Blocks: []Block{{ToolResult: &ToolResultBlock{ForID: "tu_1", Content: "dropped", IsError: false}}}},
		},
	}
	const golden = `{"max_tokens":512,"messages":[{"content":[{"text":"what now?","type":"text"}],"role":"user"},{"content":[{"text":"let me drop it","type":"text"},{"id":"tu_1","input":{"target":"axe"},"name":"drop","type":"tool_use"}],"role":"assistant"},{"content":[{"tool_use_id":"tu_1","is_error":false,"content":[{"text":"dropped","type":"text"}],"type":"tool_result"}],"role":"user"}],"model":"claude-test","system":[{"text":"you are a villager","cache_control":{"type":"ephemeral"},"type":"text"}],"tools":[{"input_schema":{"properties":{"target":{"type":"string"}},"required":["target"],"type":"object","additionalProperties":false},"name":"drop","description":"drop an item"},{"input_schema":{"properties":{},"type":"object","additionalProperties":false},"name":"muse","description":"have a thought"}]}`
	got, err := json.Marshal(a.buildParams(req))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != golden {
		t.Errorf("params drifted from golden:\n got: %s\nwant: %s", got, golden)
	}
}

// TestAnthropicErrorResultBlock (provider-wire.md §1): a tool_result carrying
// an error sets is_error true on the wire.
func TestAnthropicErrorResultBlock(t *testing.T) {
	a := &anthropicCaller{model: "m"}
	req := Request{
		MaxTokens: 100,
		Turns: []Turn{{Role: RoleUser, Blocks: []Block{
			{ToolResult: &ToolResultBlock{ForID: "tu_9", Content: "no such target", IsError: true}},
		}}},
	}
	const golden = `{"max_tokens":100,"messages":[{"content":[{"tool_use_id":"tu_9","is_error":true,"content":[{"text":"no such target","type":"text"}],"type":"tool_result"}],"role":"user"}],"model":"m"}`
	got, _ := json.Marshal(a.buildParams(req))
	if string(got) != golden {
		t.Errorf("error tool_result drifted:\n got: %s\nwant: %s", got, golden)
	}
}

// TestAnthropicNilToolsByteIdentity (provider-wire.md §5c): with Tools and
// Turns nil, buildParams reproduces the pre-feature single-message params
// byte-for-byte (both with and without a system prompt) — the regression pin
// for the untouched single-shot cloud kinds.
func TestAnthropicNilToolsByteIdentity(t *testing.T) {
	a := &anthropicCaller{model: "claude-test"}

	// preFeatureParams reproduces the exact pre-TASK-52 param construction.
	preFeatureParams := func(system, prompt string, maxTokens int64) anthropic.MessageNewParams {
		if maxTokens <= 0 {
			maxTokens = 1024
		}
		p := anthropic.MessageNewParams{
			Model:     anthropic.Model("claude-test"),
			MaxTokens: maxTokens,
			Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
		}
		if system != "" {
			p.System = []anthropic.TextBlockParam{{
				Text:         system,
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			}}
		}
		return p
	}

	cases := []Request{
		{Prompt: "hello"},
		{System: "sys", Prompt: "hello", MaxTokens: 256},
		{System: "sys", Prompt: "hello"}, // default max_tokens
	}
	for _, req := range cases {
		got, _ := json.Marshal(a.buildParams(req))
		want, _ := json.Marshal(preFeatureParams(req.System, req.Prompt, req.MaxTokens))
		if string(got) != string(want) {
			t.Errorf("nil-Tools params drifted from pre-feature:\n got: %s\nwant: %s", got, want)
		}
	}
}

// TestAnthropicParseResponse (provider-wire.md §1/§5b): canned SDK responses
// parse to ToolCalls in emission order, the concatenated text, token usage, and
// the mapped stop reason.
func TestAnthropicParseResponse(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantText  string
		wantCalls []ToolCall
		wantStop  StopReason
		wantIn    int64
		wantOut   int64
	}{
		{
			name:     "tool_use with leading text",
			raw:      `{"id":"m1","type":"message","role":"assistant","model":"claude","content":[{"type":"text","text":"let me check"},{"type":"tool_use","id":"tu_1","name":"drop","input":{"target":"axe"}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`,
			wantText: "let me check",
			wantCalls: []ToolCall{
				{ID: "tu_1", Name: "drop", Args: json.RawMessage(`{"target":"axe"}`)},
			},
			wantStop: StopToolUse,
			wantIn:   10,
			wantOut:  5,
		},
		{
			name:     "two tool_use blocks preserve emission order",
			raw:      `{"id":"m2","type":"message","role":"assistant","model":"claude","content":[{"type":"tool_use","id":"tu_a","name":"first","input":{"n":1}},{"type":"tool_use","id":"tu_b","name":"second","input":{"n":2}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":2}}`,
			wantText: "",
			wantCalls: []ToolCall{
				{ID: "tu_a", Name: "first", Args: json.RawMessage(`{"n":1}`)},
				{ID: "tu_b", Name: "second", Args: json.RawMessage(`{"n":2}`)},
			},
			wantStop: StopToolUse,
			wantIn:   1,
			wantOut:  2,
		},
		{
			name:     "plain end_turn",
			raw:      `{"id":"m3","type":"message","role":"assistant","model":"claude","content":[{"type":"text","text":"all done"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":4}}`,
			wantText: "all done",
			wantStop: StopEndTurn,
			wantIn:   3,
			wantOut:  4,
		},
		{
			name:     "max_tokens maps",
			raw:      `{"id":"m4","type":"message","role":"assistant","model":"claude","content":[{"type":"text","text":"trunc"}],"stop_reason":"max_tokens","usage":{"input_tokens":1,"output_tokens":1}}`,
			wantText: "trunc",
			wantStop: StopMaxTokens,
			wantIn:   1,
			wantOut:  1,
		},
		{
			name:     "unmapped reason falls to other",
			raw:      `{"id":"m5","type":"message","role":"assistant","model":"claude","content":[{"type":"text","text":"x"}],"stop_reason":"pause_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
			wantText: "x",
			wantStop: StopOther,
			wantIn:   1,
			wantOut:  1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var msg anthropic.Message
			if err := json.Unmarshal([]byte(c.raw), &msg); err != nil {
				t.Fatalf("unmarshal fixture: %v", err)
			}
			cr := parseAnthropicMessage(&msg)
			if cr.text != c.wantText {
				t.Errorf("text = %q, want %q", cr.text, c.wantText)
			}
			if cr.stop != c.wantStop {
				t.Errorf("stop = %q, want %q", cr.stop, c.wantStop)
			}
			if cr.inTok != c.wantIn || cr.outTok != c.wantOut {
				t.Errorf("tokens = (%d,%d), want (%d,%d)", cr.inTok, cr.outTok, c.wantIn, c.wantOut)
			}
			if len(cr.toolCalls) != len(c.wantCalls) {
				t.Fatalf("got %d tool calls, want %d", len(cr.toolCalls), len(c.wantCalls))
			}
			for i, want := range c.wantCalls {
				got := cr.toolCalls[i]
				if got.ID != want.ID || got.Name != want.Name || string(got.Args) != string(want.Args) {
					t.Errorf("call[%d] = %+v, want %+v", i, got, want)
				}
			}
		})
	}
}
