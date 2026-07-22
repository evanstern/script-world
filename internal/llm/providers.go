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
}

func newOpenAICompat(endpoint, model, apiKey, reasoningEffort string) *openaiCompat {
	return &openaiCompat{
		endpoint:        strings.TrimRight(endpoint, "/"),
		model:           model,
		apiKey:          apiKey,
		client:          &http.Client{Timeout: 120 * time.Second},
		reasoningEffort: reasoningEffort,
	}
}

func (o *openaiCompat) call(ctx context.Context, req Request) (callResult, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	var msgs []msg
	if req.System != "" {
		msgs = append(msgs, msg{Role: "system", Content: req.System})
	}
	msgs = append(msgs, msg{Role: "user", Content: req.Prompt})

	payload := map[string]any{
		"model":    o.model,
		"messages": msgs,
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
	// Structured outputs (TASK-58): when the caller supplies a schema, pin the
	// reply to it at the sampler level. Absent a schema the payload is
	// byte-identical to before — only planner calls set one.
	if len(req.ResponseSchema) > 0 {
		name := req.SchemaName
		if name == "" {
			name = "reply"
		}
		payload["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   name,
				"schema": req.ResponseSchema,
			},
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return callResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return callResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return callResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return callResult{}, fmt.Errorf("chat-completions HTTP %d: %s", resp.StatusCode, snippet)
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return callResult{}, fmt.Errorf("chat-completions response: %w", err)
	}
	if len(out.Choices) == 0 {
		return callResult{}, fmt.Errorf("chat-completions returned no choices")
	}
	return callResult{
		text:   out.Choices[0].Message.Content,
		inTok:  out.Usage.PromptTokens,
		outTok: out.Usage.CompletionTokens,
	}, nil
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

// newCloudCaller picks the cloud tier's transport from the config.
func newCloudCaller(cfg CloudConfig) caller {
	if cfg.Provider == ProviderOpenAICompat {
		return newOpenAICompat(cfg.Endpoint, cfg.Model, cfg.key(),
			resolveReasoningEffort(cfg.ReasoningEffort, ""))
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
