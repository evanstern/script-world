---
name: llm-orchestrator
description: Two-tier call layer for all model traffic — kind routing (local Ollama / cloud Anthropic-or-router), configurable N-worker local concurrency, persisted monthly spend meter with hard ceiling, circuit-breaker degraded mode, bounded-queue backpressure
kind: component
sources:
  - internal/llm/llm.go
  - internal/llm/config.go
  - internal/llm/meter.go
  - internal/llm/health.go
  - internal/llm/providers.go
verified_against: cabe1fb4fdc5fd575a58b33f4b22a184280d467d
---

# LLM orchestrator

`internal/llm` (TASK-6) is the single gateway for all model traffic. It lives entirely
**outside** the deterministic sim loop: LLM results reach the world only as recorded
inputs (TASK-7's job), so replay never re-calls a model — the determinism contract of
the substrate is structurally untouchable by inference.

## How it works

**Tiers and routing** (`llm.go`): `Kind` → tier per the grounding decisions —
`planner`, `conversation`, and `meeting` (proposal rephrasing, best-effort
flavor — [[governance]], TASK-13) go **local** (free, the only viable home for
~3,800+ calls/day); `consolidation`, `narrator`, `drama`, and `metatron` (the
gatekeeper's console turns and digests, [[metatron]]) go **cloud**. `KindMusing`
retired with spec 017: musing is no longer a scheduled call kind — it is a
roster tool a villager's planner loop may choose ([[agent-mind]], [[tool-loop]]).
The local tier
(`providers.go`) speaks OpenAI-compatible chat-completions over raw HTTP (Ollama at
`http://localhost:11434/v1`, default model `gemma4:12b-mlx` — the operator's
always-on local model); the cloud tier is provider-selectable
(`cloud.provider`, TASK-15): the default `anthropic` uses the official
`anthropic-sdk-go` against the Messages API (`claude-opus-4-8` default), with
`cache_control` on system blocks so stable prompts (souls, charters) bill at
cache-read rates on repeat calls; `openai_compat` reuses the chat-completions
caller for OpenAI-compatible routers (e.g. a LAN-local 9router), requires
`cloud.endpoint`, and pins `stream: false` because some routers stream by
default. The chat-completions body also carries `max_tokens` (from
`Request.MaxTokens`, when positive) and a per-tier `reasoning_effort`
(TASK-37): thinking-default models (gemma4 on Ollama) otherwise free-run
hidden chain-of-thought on every call — live diagnosis measured 2–6 s
calls inflated to 60–120 s, enough to saturate the tier and shed every
musing — and the compat endpoint ignores `think: false` but honors
`reasoning_effort`. The value arrives at `newOpenAICompat` already
resolved (`resolveReasoningEffort`); empty means the field is omitted
from the body. (A TASK-58 `ResponseSchema`/`SchemaName` structured-output
path — a sampler-level `response_format {type: json_schema}` envelope on
schema-carrying requests — lost its one caller when spec 017 moved the
planner to native tools, and was deleted as dead code in TASK-71; git
history has it if a schema-constrained single-shot kind ever returns.)

**Agent tool-use loop transport** (`llm.go`/`providers.go`, TASK-52, spec
017; every field below is additive — a request that sets none of them
marshals byte-identical to before): `Request.Tools` (`[]ToolDecl{Name,
Description, InputSchema}`) declares the tools a call may invoke this round;
`Request.Turns` (`[]Turn{Role, Blocks}`, a `Block` one of text /
`ToolUseBlock` / `ToolResultBlock`) is an ephemeral multi-turn transcript
that replaces `Prompt` as the message source when non-nil (never persisted,
never replayed); `Request.SkipObserve` marks a loop-internal per-round
`Submit` so the worker feeds no fractional per-call sample to the tier
estimator. `Response.ToolCalls` (`[]ToolCall{ID, Name, Args}`) carries the
calls a reply emitted, in emission order; `Response.Stop` (`StopReason`:
`end_turn` / `tool_use` / `max_tokens` / `other`) is the mapped provider stop
reason [[tool-loop]]'s driver reads to tell "model finished" from "model
wants results". Both providers translate their native wire shape: the
Anthropic caller (`anthropicCaller.buildParams`) sends `req.Tools` as native
`ToolUnionParam`s and `req.Turns` as `tool_use`/`tool_result` content blocks
— `anthropicInputSchema` decodes a derived JSON Schema into the SDK's typed
`ToolInputSchemaParam`, round-tripping any keyword the SDK's own struct
doesn't model (e.g. `additionalProperties`) through `ExtraFields`, which the
SDK's `UnmarshalJSON` would otherwise drop. `openaiCompat.call` picks a path
per the tier's resolved `tool_mode` (below): `callNative` sends
OpenAI-style `tools`/`tool_calls` (a nil-`Tools` nil-`Turns` request is
byte-identical to the pre-feature body); `callJSON` is the FR-010 fallback
for backends whose native function calling is unreliable — it appends a
rendered tool catalog to the system prompt, grammar-constrains every reply
to a flat envelope (`{"tool": <name|"none">, "args": <object>, "say":
<text>}`) via `response_format`, and synthesizes a per-round call ID
(`"env-<round>"`, the count of assistant turns already in the transcript)
since the envelope carries none — a fallback-mode transcript must keep
exactly one assistant turn per round or synthesized IDs collide.

**Tool-call strategy** (`config.go`, TASK-52): `LocalConfig.ToolMode` /
`CloudConfig.ToolMode` (`"tool_mode"` in `llm.json`, per-tier, mirroring
`ReasoningEffort`'s shape) select `ToolModeNative` (default, absent/`""`) or
`ToolModeJSON`; `ToolModeResolved()` normalizes an unknown value back to
native with an operator warning (never an error, same clamp doctrine as
`Workers()`). The cloud knob applies only to the `openai_compat` provider —
the Anthropic SDK path is always native and ignores it. `Config.LoopMaxRounds`
(`"loop_max_rounds"`, top-level) is the tool-use loop's hard per-cognition
round cap; `Config.Rounds()` normalizes it the same warn-not-error way
`LocalConfig.Workers()` does: absent/0 → 8 (`defaultLoopMaxRounds`), 1–16
pass through, negative clamps to 8 with a warning, and above 16
(`maxLoopMaxRounds`) clamps to 16 with a warning.

**Whole-loop latency feed** (`ObserveCognition`, TASK-52): the tool-use loop
issues several per-round `Submit` calls (each `SkipObserve: true`, so none
individually reaches the estimator) and reports exactly one whole-cognition
wall time via `Orchestrator.ObserveCognition(kind, totalMillis)` when it
finishes. `ObserveCognition` resolves the kind's tier and calls the same
`feedEstimate` helper the per-call worker path uses, so both feeding paths
normalize by the kind's registered point cost and fire the same recalibrate
hook identically — [[tool-loop]]'s `Run` calls this only on a completed
termination (landed / model_done / cap_exhausted), never on the failure
family (admission_refused / provider_error / ctx_done), mirroring the
worker's own successes-only doctrine below.

**Concurrency** (TASK-45): each tier owns `slots` worker goroutines — N identical
copies of the same worker loop draining the same two channels. The local tier's
slot count comes from `llm.json`'s `local.parallel` via `LocalConfig.Workers()`
(absent/0 → 1, clamped to `maxLocalWorkers` = 16, negative → 1; never an error —
the daemon prints the clamp warning at boot and the world always starts); the
cloud tier is pinned at 1. An `atomic.Int32` `inflight` counter per tier
(incremented at dequeue, decremented on every reply path) tracks occupied slots
for admission.

**Priority lanes**: conversations (`KindConversation`) ride a per-tier priority
queue idle workers drain first — dialogue turns are interactive, while planner
thoughts tolerate staleness (the reflex grace covers them). The opposite
extreme is caller-flagged: `Request.BestEffort` calls are refused with
`ErrTierBusy` when no worker slot is free — either local queue has work
waiting, or `inflight` has reached `slots` — flavor yields to real cognition.
The flag belongs to the caller, not the orchestrator (`meeting.go`'s
best-effort proposal rephrasing is the current user); scheduled musing was
its first caller until spec 017 folded musing into the planner's tool-use
loop as an ordinary roster choice ([[agent-mind]], [[tool-loop]]) — the
mechanism itself, including the caller-owned fairness-floor doctrine, is
unchanged and stands ready for any future drop-when-busy kind. A worker-side hard cap
(`workerCallCap`, 2 min) bounds any single provider call so a hung transport can
never wedge a tier. **Submit** is synchronous with immediate admission control, each failure mode a
distinct error: `ErrBudgetExhausted` (cloud ceiling reached — checked BEFORE any
HTTP), `ErrTierDown` (circuit open — fails fast, no hang), `ErrQueueFull` (bounded
per-tier queue of 32 saturated). That backpressure surface is what will let local
throughput cap effective sim speed when TASK-7 wires minds in.

**Spend meter** (`meter.go`): actual per-call cost from configured pricing
(`input_usd_per_mtok` / `output_usd_per_mtok`), accumulated per UTC month and
persisted in the store's meta table (`llm_spend_YYYY-MM`) — restarts never forget
money spent. The ceiling **throttles rather than silently overspending**: refusal
happens at admission, and the local tier is unaffected.

**Degraded mode** (`health.go`): a per-tier circuit breaker — 3 consecutive failures
open it (15 s backoff doubling to 5 min), an open circuit refuses instantly, one
half-open probe tests recovery, success resets. Busy is not down (TASK-22 live
finding): the worker skips queued jobs whose caller already gave up (no model
call, no health strike) and never counts a failure when the caller's own ctx
died mid-call — only genuine provider failures and the worker cap strike the
breaker, so planners timing out behind a long conversation can no longer
self-inflict an outage. A killed model degrades the AI layer;
the daemon and loop never notice.

**Latency estimation** (TASK-32, [[cognition]]): each tier carries a live
`cognition.Estimator` of seconds-per-point — the worker is the one place every
call's true duration is observed, so on each *successful* call it samples the
wall time normalized by the kind's registered point cost
(`cognition.ClassForKind`; failures are not latency observations, and spike
rejection guards only the high side). With `local.parallel` > 1 those samples
are per-call wall times under concurrent load — server-side contention included
— so the estimate converges on true concurrent-rate seconds-per-point rather
than a serial-calibration optimum. Estimators start from bootstrap seeds
(`cognition.SeedFor`); `SeedCalibration` re-seeds both tiers from a
calibration profile once at daemon start, and `SecondsPerPoint` exposes the
live estimate — the router's bridge from Fibonacci points to this
deployment's wall clock, read by the mind when routing. When an estimator
first breaches the spike-rate drift threshold, the hook installed via
`SetRecalibrateHook` fires (own goroutine, once per breach episode); the mind
turns it into a `cog.recalibration_recommended` telemetry event. Two small
exports serve the same layer: `TierFor(kind)` lets the mind read the right
tier's estimate, and `Kinds()` returns every accepted call kind sorted — the
cognition registry's completeness gate iterates it at daemon start so an
unregistered kind can never reach a model at runtime.

**Config** (`config.go`): `llm.json` in the save directory, written with defaults by
`promptworld new`; deleting the file disables the orchestrator entirely. Hosted-API
keys are never stored — only the *name* of an environment variable (`api_key_env`,
default `ANTHROPIC_API_KEY`). The one exception is the optional inline `api_key`
(both tiers), intended solely for keys that guard LAN-local routers; when both are
set the inline key wins. Provider values are validated at load time (`LoadConfig`
rejects unknown providers and `openai_compat` without an endpoint; the local
`parallel` field is deliberately exempt — out-of-range values clamp with a
warning instead of failing the boot). Both tiers
carry an optional `reasoning_effort` (`*string`, TASK-37) with a nil/""
convention resolved by `resolveReasoningEffort`: local absent defaults to
`"none"` (interiority prose never needs hidden reasoning, and local latency is
the cap on sim speed), while an explicit `""` sends nothing — the escape hatch
for backends that reject the field; cloud absent or `""` sends nothing, and the
field only applies on the `openai_compat` transport (the Anthropic SDK path is
untouched).

## Connections

[[daemon-lifecycle]] starts it when config exists; [[ipc-server]] exposes `llm_call`
and folds `StatusSnapshot` into the protocol status; [[cli-promptworld]]'s `llm`
subcommand is the one-shot proof path; the [[tui-client]] metatron pane displays tier
health and spend; the meter persists via [[event-log]]'s store (meta table). TASK-7
(agent minds), TASK-9 (consolidation), TASK-11 (narrator), and TASK-12 (Metatron)
are the intended callers. [[tool-loop]] is the transport's other consumer (spec 017):
it drives `Request.Tools`/`Turns`/`SkipObserve` and `Response.ToolCalls`/`Stop`
through `Submit`, and reports whole-cognition latency via `ObserveCognition` — used
by both [[agent-mind]]'s `runPlan` and [[metatron]]'s `Turn`.

## Operational notes

Tested against httptest mock providers for both tiers: routing, cost math, ceiling
refusal with zero HTTP, circuit open/fast-fail/recovery, queue overflow, meter
persistence, and world-keeps-ticking-with-dead-endpoints; concurrency is proven
under `go test -race` (4-wide overlapping in-flight calls, slot-aware best-effort
admission, exactly-once breaker/meter/estimator accounting under parallel load,
serial-when-absent compatibility). Live-verified against a
real Ollama instance end-to-end through the daemon. Motivating measurement
(TASK-45): one worker serialized everything — 130 s queue waits behind 19 s calls
produced rejected-stale planners and total musing silence, while the server ran
4 concurrent calls in 0.98 s wall vs 3.8 s for one cold call. Budget reality check: nightly
consolidation volume at v1 scale ≈ $34/month on the default cloud model — inside the
$100 ceiling from the grounding session.
