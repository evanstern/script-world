package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// caller is one tier's transport. It returns the fields a transport owns for
// one provider call (text, any tool calls, stop reason, token counts); the
// worker completes the Response with tier/model/cost/millis (TASK-52).
type caller interface {
	call(ctx context.Context, req Request) (callResult, error)
}

// callResult is one provider call's transport-owned output. Its zero value is
// a valid empty reply (no text, no tool calls, StopReason ""), so the
// single-shot path never has to reason about tool fields.
type callResult struct {
	text      string
	toolCalls []ToolCall
	stop      StopReason
	inTok     int64
	outTok    int64
}

// --- OpenAI-compatible chat completions (Ollama, 9router, et al.) ---
// Serves the local tier always, and the cloud tier when
// cloud.provider = "openai_compat".

type openaiCompat struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
	// reasoningEffort is already resolved (see resolveReasoningEffort);
	// empty means omit the field from the request body entirely.
	reasoningEffort string
	// toolMode is the resolved tool-call strategy (ToolModeNative default, or
	// ToolModeJSON for the fallback envelope). Set from config at construction;
	// only matters once a Request carries Tools (TASK-52).
	toolMode string
}

func newOpenAICompat(endpoint, model, apiKey, reasoningEffort, toolMode string) *openaiCompat {
	if toolMode == "" {
		toolMode = ToolModeNative
	}
	return &openaiCompat{
		endpoint:        strings.TrimRight(endpoint, "/"),
		model:           model,
		apiKey:          apiKey,
		client:          &http.Client{Timeout: 120 * time.Second},
		reasoningEffort: reasoningEffort,
		toolMode:        toolMode,
	}
}

// oaiMessage is one chat-completions message. Its first two fields match the
// pre-feature single-shot message exactly, so a nil-Turns request marshals
// byte-identically; the omitempty tool fields carry the native tool exchange
// (TASK-52).
type oaiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

// oaiToolCall is an assistant-side native function call replayed in the
// transcript. function.arguments is a JSON-ENCODED STRING per the OpenAI wire.
type oaiToolCall struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Function oaiFunc `json:"function"`
}

type oaiFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// oaiResponse decodes a chat-completions reply for both modes: content and any
// native tool_calls, plus the finish reason and usage.
type oaiResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
}

func (o *openaiCompat) call(ctx context.Context, req Request) (callResult, error) {
	if o.toolMode == ToolModeJSON && len(req.Tools) > 0 {
		return o.callJSON(ctx, req)
	}
	return o.callNative(ctx, req)
}

// callNative is the default path: OpenAI-compatible function calling
// (provider-wire.md §2). With no Tools/Turns it produces the exact pre-feature
// request body (byte-identical for the untouched single-shot kinds); the json
// fallback is also routed here whenever a request carries no tools, so a plain
// reply never pays for the envelope machinery.
func (o *openaiCompat) callNative(ctx context.Context, req Request) (callResult, error) {
	payload := map[string]any{
		"model":    o.model,
		"messages": o.nativeMessages(req),
		// Some routers (9router) stream by default; this decoder wants one
		// JSON object, so pin it.
		"stream": false,
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if o.reasoningEffort != "" {
		payload["reasoning_effort"] = o.reasoningEffort
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, td := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        td.Name,
					"description": td.Description,
					"parameters":  td.InputSchema,
				},
			})
		}
		payload["tools"] = tools
	}

	out, err := o.do(ctx, payload)
	if err != nil {
		return callResult{}, err
	}
	choice := out.Choices[0]
	cr := callResult{
		text:   choice.Message.Content,
		stop:   mapFinishReason(choice.FinishReason),
		inTok:  out.Usage.PromptTokens,
		outTok: out.Usage.CompletionTokens,
	}
	for _, tc := range choice.Message.ToolCalls {
		args := json.RawMessage(tc.Function.Arguments) // arguments is a JSON string; decode to raw
		if len(args) == 0 {
			args = json.RawMessage("{}")
		}
		cr.toolCalls = append(cr.toolCalls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Args: args})
	}
	return cr, nil
}

// nativeMessages renders the transcript for native mode: assistant turns carry
// tool_calls (arguments JSON-encoded as a string); tool_result blocks become
// role:"tool" messages keyed by tool_call_id; text blocks stay plain. A nil
// Turns request is the pre-feature system?+user(Prompt) pair, byte-identical.
func (o *openaiCompat) nativeMessages(req Request) []oaiMessage {
	var msgs []oaiMessage
	if req.System != "" {
		msgs = append(msgs, oaiMessage{Role: "system", Content: req.System})
	}
	if len(req.Turns) == 0 {
		msgs = append(msgs, oaiMessage{Role: "user", Content: req.Prompt})
		return msgs
	}
	for _, turn := range req.Turns {
		if turn.Role == RoleAssistant {
			m := oaiMessage{Role: "assistant"}
			var text strings.Builder
			for _, b := range turn.Blocks {
				if b.ToolUse != nil {
					m.ToolCalls = append(m.ToolCalls, oaiToolCall{
						ID:       b.ToolUse.ID,
						Type:     "function",
						Function: oaiFunc{Name: b.ToolUse.Name, Arguments: string(b.ToolUse.Args)},
					})
				} else {
					text.WriteString(b.Text)
				}
			}
			m.Content = text.String()
			msgs = append(msgs, m)
			continue
		}
		// User turn: each tool_result is its own role:"tool" message; text
		// blocks coalesce into one user message.
		var text strings.Builder
		for _, b := range turn.Blocks {
			if b.ToolResult != nil {
				msgs = append(msgs, oaiMessage{Role: "tool", ToolCallID: b.ToolResult.ForID, Content: b.ToolResult.Content})
			} else {
				text.WriteString(b.Text)
			}
		}
		if text.Len() > 0 {
			msgs = append(msgs, oaiMessage{Role: "user", Content: text.String()})
		}
	}
	return msgs
}

// callJSON is the FR-010 fallback (provider-wire.md §3): tools are described in
// the system prompt, every round is grammar-constrained to a small envelope via
// response_format, and tool results ride back as plain user turns. Exactly one
// tool call per round by construction.
func (o *openaiCompat) callJSON(ctx context.Context, req Request) (callResult, error) {
	payload := map[string]any{
		"model":    o.model,
		"messages": jsonModeMessages(req),
		"stream":   false,
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "tool_call",
				"schema": envelopeSchema(req.Tools),
			},
		},
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if o.reasoningEffort != "" {
		payload["reasoning_effort"] = o.reasoningEffort
	}

	out, err := o.do(ctx, payload)
	if err != nil {
		return callResult{}, err
	}
	choice := out.Choices[0]
	cr := callResult{inTok: out.Usage.PromptTokens, outTok: out.Usage.CompletionTokens}

	var env struct {
		Tool string          `json:"tool"`
		Args json.RawMessage `json:"args"`
		Say  string          `json:"say"`
	}
	if err := json.Unmarshal([]byte(choice.Message.Content), &env); err != nil {
		return callResult{}, fmt.Errorf("tool-call envelope: %w", err)
	}
	cr.text = env.Say
	if env.Tool == "" || env.Tool == "none" {
		cr.stop = StopEndTurn
		return cr, nil
	}
	args := env.Args
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	cr.stop = StopToolUse
	// Synthesize a per-round ID: the envelope carries none, so derive a stable
	// ordinal from the count of assistant turns already in the transcript (this
	// reply is the next one). Unique within a cognition, deterministic on
	// replay of the same transcript.
	cr.toolCalls = []ToolCall{{ID: fmt.Sprintf("env-%d", jsonModeRound(req.Turns)), Name: env.Tool, Args: args}}
	return cr, nil
}

// jsonModeRound is the 1-based ordinal of the reply now being requested: one
// past the number of assistant turns already recorded in the transcript.
func jsonModeRound(turns []Turn) int {
	round := 1
	for _, t := range turns {
		if t.Role == RoleAssistant {
			round++
		}
	}
	return round
}

// jsonModeMessages renders the transcript for the fallback envelope: the system
// prompt gains the tool catalog; tool_result blocks become plain user text
// ("Tool result/error (<name>): <content>"); assistant tool calls echo back as
// the envelope the model produced. No role:"tool" messages in this mode.
func jsonModeMessages(req Request) []oaiMessage {
	byID := toolNamesByID(req.Turns)
	msgs := []oaiMessage{{Role: "system", Content: renderToolSystem(req.System, req.Tools)}}
	if len(req.Turns) == 0 {
		msgs = append(msgs, oaiMessage{Role: "user", Content: req.Prompt})
		return msgs
	}
	for _, turn := range req.Turns {
		if turn.Role == RoleAssistant {
			msgs = append(msgs, oaiMessage{Role: "assistant", Content: jsonModeAssistant(turn)})
			continue
		}
		var text strings.Builder
		for _, b := range turn.Blocks {
			if b.ToolResult != nil {
				label := "Tool result"
				if b.ToolResult.IsError {
					label = "Tool error"
				}
				if text.Len() > 0 {
					text.WriteString("\n")
				}
				fmt.Fprintf(&text, "%s (%s): %s", label, byID[b.ToolResult.ForID], b.ToolResult.Content)
			} else {
				if text.Len() > 0 {
					text.WriteString("\n")
				}
				text.WriteString(b.Text)
			}
		}
		msgs = append(msgs, oaiMessage{Role: "user", Content: text.String()})
	}
	return msgs
}

// jsonModeAssistant reconstructs the envelope an assistant turn produced, so
// the replayed transcript mirrors what the constrained sampler emitted.
func jsonModeAssistant(turn Turn) string {
	env := map[string]any{}
	var say strings.Builder
	tool := "none"
	for _, b := range turn.Blocks {
		if b.ToolUse != nil {
			tool = b.ToolUse.Name
			if len(b.ToolUse.Args) > 0 {
				env["args"] = b.ToolUse.Args
			}
		} else {
			say.WriteString(b.Text)
		}
	}
	env["tool"] = tool
	if say.Len() > 0 {
		env["say"] = say.String()
	}
	b, _ := json.Marshal(env)
	return string(b)
}

// toolNamesByID maps each recorded tool_use ID to its tool name so a later
// tool_result (which carries only the ID) can be labeled by name.
func toolNamesByID(turns []Turn) map[string]string {
	m := map[string]string{}
	for _, t := range turns {
		for _, b := range t.Blocks {
			if b.ToolUse != nil {
				m[b.ToolUse.ID] = b.ToolUse.Name
			}
		}
	}
	return m
}

// renderToolSystem appends a deterministic tool catalog to the base system
// prompt: one line per tool (name, gloss, compact args schema) plus the
// envelope contract. Order follows the roster, so the cached prefix is stable.
func renderToolSystem(base string, tools []ToolDecl) string {
	var b strings.Builder
	if base != "" {
		b.WriteString(base)
		b.WriteString("\n\n")
	}
	b.WriteString(`Call one tool per reply. Respond ONLY with a JSON object {"tool": <tool name or "none">, "args": <object>, "say": <text>}.`)
	b.WriteString("\nAvailable tools:\n")
	for _, td := range tools {
		b.WriteString("- ")
		b.WriteString(td.Name)
		if td.Description != "" {
			b.WriteString(": ")
			b.WriteString(td.Description)
		}
		b.WriteString("\n  args schema: ")
		b.Write(td.InputSchema)
		b.WriteString("\n")
	}
	b.WriteString(`Set "tool" to "none" when you have no tool to call; put any final message in "say".`)
	return b.String()
}

// envelopeSchema is the flat, grammar-friendly response schema for the fallback
// (provider-wire.md §3): a tool-name enum (declared names + "none"), a free
// args object, and a bounded say string. Per-tool arg schemas are validated
// driver-side, not inlined here (llama.cpp grammar reliability).
func envelopeSchema(tools []ToolDecl) json.RawMessage {
	names := make([]string, 0, len(tools)+1)
	for _, td := range tools {
		names = append(names, td.Name)
	}
	names = append(names, "none")
	b, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tool": map[string]any{"type": "string", "enum": names},
			"args": map[string]any{"type": "object"},
			"say":  map[string]any{"type": "string", "maxLength": 400},
		},
		"required":             []string{"tool"},
		"additionalProperties": false,
	})
	return b
}

// mapFinishReason collapses the OpenAI finish_reason onto our StopReason.
func mapFinishReason(r string) StopReason {
	switch r {
	case "tool_calls":
		return StopToolUse
	case "stop":
		return StopEndTurn
	case "length":
		return StopMaxTokens
	default:
		return StopOther
	}
}

// do marshals the payload, posts it, and decodes the reply — the HTTP mechanics
// shared by both modes.
func (o *openaiCompat) do(ctx context.Context, payload map[string]any) (oaiResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return oaiResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return oaiResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return oaiResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return oaiResponse{}, fmt.Errorf("chat-completions HTTP %d: %s", resp.StatusCode, snippet)
	}
	var out oaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return oaiResponse{}, fmt.Errorf("chat-completions response: %w", err)
	}
	if len(out.Choices) == 0 {
		return oaiResponse{}, fmt.Errorf("chat-completions returned no choices")
	}
	return out, nil
}

// --- cloud tier: Anthropic Messages API via the official SDK ---

type anthropicCaller struct {
	client anthropic.Client
	model  string
}

func newAnthropicCaller(cfg CloudConfig) *anthropicCaller {
	var opts []option.RequestOption
	if key := cfg.key(); key != "" {
		opts = append(opts, option.WithAPIKey(key))
	}
	if cfg.Endpoint != "" {
		opts = append(opts, option.WithBaseURL(cfg.Endpoint))
	}
	return &anthropicCaller{client: anthropic.NewClient(opts...), model: cfg.Model}
}

// newCloudCaller picks the cloud tier's transport from the config. The
// Anthropic path is always native and ignores tool_mode; only the openai_compat
// cloud router honors it (TASK-52).
func newCloudCaller(cfg CloudConfig) caller {
	if cfg.Provider == ProviderOpenAICompat {
		mode, _ := cfg.ToolModeResolved()
		return newOpenAICompat(cfg.Endpoint, cfg.Model, cfg.key(),
			resolveReasoningEffort(cfg.ReasoningEffort, ""), mode)
	}
	return newAnthropicCaller(cfg)
}

func (a *anthropicCaller) call(ctx context.Context, req Request) (callResult, error) {
	resp, err := a.client.Messages.New(ctx, a.buildParams(req))
	if err != nil {
		return callResult{}, err
	}
	return parseAnthropicMessage(resp), nil
}

// buildParams translates a Request into Anthropic Messages params (TASK-52,
// provider-wire.md §1). Turns become the messages transcript (assistant
// tool_use echoes, user tool_result outcomes); nil Turns falls back to the
// single Prompt user message, byte-identical to the pre-feature request. Tools
// become the native tools parameter; nil Tools sends no tools field. The
// system prompt keeps its ephemeral cache breakpoint so per-kind-stable tool
// declarations ride the cached prefix across rounds.
func (a *anthropicCaller) buildParams(req Request) anthropic.MessageNewParams {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: maxTokens,
	}
	if len(req.Turns) > 0 {
		msgs := make([]anthropic.MessageParam, 0, len(req.Turns))
		for _, turn := range req.Turns {
			msgs = append(msgs, anthropicTurn(turn))
		}
		params.Messages = msgs
	} else {
		params.Messages = []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(req.Prompt))}
	}
	if req.System != "" {
		// Stable system prompts (agent souls, the narrator charter) are the
		// cacheable prefix — mark them so repeat calls bill at cache-read
		// rates.
		params.System = []anthropic.TextBlockParam{{
			Text:         req.System,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}}
	}
	if len(req.Tools) > 0 {
		tools := make([]anthropic.ToolUnionParam, 0, len(req.Tools))
		for _, td := range req.Tools {
			tp := anthropic.ToolParam{
				Name:        td.Name,
				InputSchema: anthropicInputSchema(td.InputSchema),
			}
			if td.Description != "" {
				tp.Description = anthropic.String(td.Description)
			}
			tools = append(tools, anthropic.ToolUnionParam{OfTool: &tp})
		}
		params.Tools = tools
	}
	return params
}

// anthropicTurn maps one transcript Turn to an Anthropic message. A block is
// exactly one of tool_use (assistant call echo), tool_result (user outcome),
// or text.
func anthropicTurn(turn Turn) anthropic.MessageParam {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(turn.Blocks))
	for _, b := range turn.Blocks {
		switch {
		case b.ToolUse != nil:
			blocks = append(blocks, anthropic.NewToolUseBlock(b.ToolUse.ID, b.ToolUse.Args, b.ToolUse.Name))
		case b.ToolResult != nil:
			blocks = append(blocks, anthropic.NewToolResultBlock(b.ToolResult.ForID, b.ToolResult.Content, b.ToolResult.IsError))
		default:
			blocks = append(blocks, anthropic.NewTextBlock(b.Text))
		}
	}
	if turn.Role == RoleAssistant {
		return anthropic.NewAssistantMessage(blocks...)
	}
	return anthropic.NewUserMessage(blocks...)
}

// anthropicInputSchema decodes a derived JSON Schema object (tool.InputSchema)
// into the SDK's typed input_schema param, preserving every keyword — including
// additionalProperties — via ExtraFields, which the SDK's own UnmarshalJSON
// drops. The properties/required/type keys map to the typed fields; anything
// else rides ExtraFields.
func anthropicInputSchema(raw json.RawMessage) anthropic.ToolInputSchemaParam {
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	var is anthropic.ToolInputSchemaParam
	if p, ok := m["properties"]; ok {
		var v any
		_ = json.Unmarshal(p, &v)
		is.Properties = v
	}
	if r, ok := m["required"]; ok {
		var v []string
		_ = json.Unmarshal(r, &v)
		is.Required = v
	}
	extra := map[string]any{}
	for k, v := range m {
		switch k {
		case "type", "properties", "required":
			continue
		}
		var val any
		_ = json.Unmarshal(v, &val)
		extra[k] = val
	}
	if len(extra) > 0 {
		is.ExtraFields = extra
	}
	return is
}

// parseAnthropicMessage extracts text, tool calls (in emission order), token
// usage, and the mapped stop reason from an Anthropic response.
func parseAnthropicMessage(resp *anthropic.Message) callResult {
	var text strings.Builder
	var calls []ToolCall
	for _, block := range resp.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			text.WriteString(b.Text)
		case anthropic.ToolUseBlock:
			calls = append(calls, ToolCall{ID: b.ID, Name: b.Name, Args: json.RawMessage(b.Input)})
		}
	}
	return callResult{
		text:      text.String(),
		toolCalls: calls,
		stop:      mapAnthropicStop(resp.StopReason),
		inTok:     resp.Usage.InputTokens,
		outTok:    resp.Usage.OutputTokens,
	}
}

// mapAnthropicStop collapses the SDK's stop_reason enum onto our StopReason.
func mapAnthropicStop(r anthropic.StopReason) StopReason {
	switch r {
	case anthropic.StopReasonToolUse:
		return StopToolUse
	case anthropic.StopReasonEndTurn:
		return StopEndTurn
	case anthropic.StopReasonMaxTokens:
		return StopMaxTokens
	default:
		return StopOther
	}
}
