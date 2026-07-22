# Contract: Provider wire shapes (spec 017)

How each transport encodes the tool-call exchange. The loop driver is wire-agnostic: a
caller translates `Request{Tools, Turns}` out and `Response{ToolCalls, Stop}` back, and
the driver cannot tell which shape produced a call (FR-010's "fallback satisfies
FR-003–FR-008" falls out of this).

## 1. Anthropic Messages (cloud, always native) — `anthropicCaller`

- `Request.Tools` → Messages `tools: [{name, description, input_schema}]`.
- `Request.Turns` → `messages`: assistant `tool_use` blocks echo prior calls; user
  `tool_result` blocks (`tool_use_id`, `content`, `is_error`) carry outcomes. First
  round: single user message (Seed), exactly today's shape when Tools is nil.
- `Response.ToolCalls` ← `tool_use` content blocks (id, name, input) in emission
  order; `Response.Stop` ← `stop_reason` (`tool_use` → StopToolUse, `end_turn` →
  StopEndTurn).
- System prompt keeps `CacheControlEphemeral` (providers.go:141 pattern); tool
  declarations are per-kind stable so the cached prefix stays warm across rounds.
- `ResponseSchema` remains ignored by this caller (unchanged).

## 2. OpenAI-compatible, `tool_mode: "native"` (local default) — `openaiCompat`

- `Request.Tools` → `tools: [{type:"function", function:{name, description,
  parameters}}]`.
- `Request.Turns` → `messages`: assistant turns carry `tool_calls:
  [{id, type:"function", function:{name, arguments}}]`; results are
  `{role:"tool", tool_call_id, content}` messages.
- `Response.ToolCalls` ← `choices[0].message.tool_calls` (note: `function.arguments`
  is a JSON-encoded STRING — decode to RawMessage); `Stop` ← `finish_reason`
  (`tool_calls` → StopToolUse, `stop` → StopEndTurn).
- `reasoning_effort` passthrough unchanged. `response_format` MUST NOT be sent in
  native mode.

## 3. Fallback envelope, `tool_mode: "json"` (per-model config) — `openaiCompat`

The documented FR-010 fallback: grammar-constrained structured output emulating one
tool-call round per reply. This generalizes the proven `plannerReplySchema` path
(parse.go:122 hardening notes apply: flat schema, explicit `required`, `maxLength`
bounds, enum for tool names).

- Tool declarations render into the SYSTEM prompt (name, gloss, parameter shapes —
  derived from the same `tool.InputSchema` source as native mode).
- Every round sends `response_format: {type:"json_schema", json_schema:{name:
  "tool_call", schema: ENVELOPE}}` where ENVELOPE is:

```json
{
  "type": "object",
  "properties": {
    "tool":  {"type": "string", "enum": ["<declared tool names>", "none"]},
    "args":  {"type": "object"},
    "say":   {"type": "string", "maxLength": 400}
  },
  "required": ["tool"],
  "additionalProperties": false
}
```

- `tool != "none"` → one `Response.ToolCall` (`ID` synthesized `"env-<round>"`,
  `Args` = `args`), `Stop` = StopToolUse. `tool == "none"` → final answer (`say` is
  the closing text), `Stop` = StopEndTurn.
- Tool results feed back as a plain user turn: `Tool result (<name>): <content>` (or
  `Tool error (<name>): …`) — no `role:"tool"` messages in this mode.
- Per-tool `args` sub-schema is NOT inlined into the envelope (llama.cpp grammar
  reliability: keep the constrained schema small); args are validated driver-side
  against `tool.InputSchema` and malformed args come back as `rejected_malformed`
  results the model can repair within the cap.
- Exactly one tool call per round in this mode by construction.

## 4. Mode selection & failure semantics

- Mode is static per (tier, model) from config (`local.tool_mode` / `cloud.tool_mode`,
  default native; Anthropic ignores the knob — always native). No runtime auto-detect
  or mid-cognition switching: deterministic, auditable behavior per world config.
- A native-mode model that replies with plain text despite declared tools is a valid
  `end_turn` (the model chose not to act) — FR-015's recorded failure outcome comes
  from the driver seeing no landed acting call, not from the transport.
- Transport-level errors (HTTP, SDK, decode) surface as Submit errors exactly as
  today — one strike against the tier's breaker, loop terminates `provider_error`.

## 5. Fixture round-trip tests (test contract §2)

For each mode: recorded request/response fixtures assert (a) outgoing JSON matches the
shapes above byte-for-byte given a fixed Request, (b) incoming fixtures parse to the
same `ToolCalls`/`Stop` regardless of mode, (c) nil-Tools requests are byte-identical
to pre-feature requests (regression pin for the five untouched single-shot kinds).
