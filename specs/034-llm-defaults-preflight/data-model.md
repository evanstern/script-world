# Data model — spec 034

## ProviderCondition (in-memory, per provider; exported on the status wire)

One condition slot per named provider, held on the orchestrator's `provider`
struct beside the circuit breaker.

| Field | Type | Meaning |
|-------|------|---------|
| `kind` | enum | `model-missing` \| `endpoint-unreachable` \| `tool-silent`; empty = healthy |
| `detail` | string | evidence, e.g. `model "cogito:3b" not served by http://localhost:11434/v1` or `8 consecutive tool-free completions` |
| `remedy` | string | operator action, e.g. `ollama pull cogito:3b`, `start the model server`, `set providers.local.tool_mode to "json" and restart` |
| `since` | timestamp | when the condition was raised (for log/event context) |

**Precedence** (one slot, dominant problem wins):
`endpoint-unreachable` > `model-missing` > `tool-silent`. A preflight-raised
condition is never overwritten by the tool-silence detector; a re-probe may
upgrade `model-missing` → `endpoint-unreachable` and vice versa.

**State transitions**

```
                    probe: conn/timeout error          probe: model absent from listing
        healthy ────────────────────────▶ unreachable ◀──────────────────▶ model-missing
           ▲  ▲                                │      (re-probe reclassifies)     │
           │  │                                └──────────────┬───────────────────┘
           │  │ worker: ≥8 consecutive tool-free               │ re-probe OK (60s cadence
           │  │ completions on tool-carrying calls             │ while active), or any
           │  └────────────▶ tool-silent                       │ successful call
           │                     │                             │
           └─────────────────────┴─────────────────────────────┘
             clear: any successful call with a tool call landing (tool-silent)
             / any provider traffic success or passing re-probe (preflight kinds)
```

Every transition (raise, reclassify, clear) fires the condition hook →
daemon log line + `daemon.llm_warning` event.

## Tool-silence counter (in-memory, per provider)

| Field | Type | Rule |
|-------|------|------|
| `consecutiveToolFree` | int | +1 on completed call where `len(req.Tools) > 0` and zero tool calls returned; reset to 0 when any tool call returns; untouched by non-tool calls and by transport failures |

Threshold: 8 (research R4). At threshold, raise `tool-silent` unless a
preflight condition already occupies the slot.

## ProviderStatus (wire extension — `internal/llm`, rides `StatusData.LLM`)

Existing struct (llm.go:264-274) gains omitempty fields:

| New field | JSON | Content |
|-----------|------|---------|
| `Condition` | `condition,omitempty` | the condition kind, empty when healthy |
| `ConditionDetail` | `condition_detail,omitempty` | `detail` above |
| `ConditionRemedy` | `condition_remedy,omitempty` | `remedy` above |

Absent fields = healthy: existing `status --json` consumers see no change on
healthy worlds.

## `daemon.llm_warning` event (durable, broadcast)

Appended via the existing daemon-event pattern (`daemon.started` precedent).
Payload:

| Field | Type | Meaning |
|-------|------|---------|
| `provider` | string | provider name |
| `kind` | string | condition kind, or the kind being cleared |
| `detail` | string | evidence text |
| `remedy` | string | operator action (empty on clear) |
| `active` | bool | true = raised/reclassified, false = cleared |

Operator-facing only: whitelisted with a no-op reducer arm (never mutates world
state), never rendered in chronicle/narrator surfaces.

## Fresh-world default LLM config (durable artifact — `llm.json`)

Written by `promptworld new` via `WriteDefault`/`DefaultConfig`. Local provider
changes only:

| Field | Old | New |
|-------|-----|-----|
| `model` | `gemma4:12b-mlx` | `cogito:3b` |
| `tool_mode` | (absent → native) | `"json"` |
| `parallel` | (absent → 1) | `4` |

Cloud provider, routes, and budget unchanged. Existing worlds' `llm.json`
files are never rewritten (FR-010).
