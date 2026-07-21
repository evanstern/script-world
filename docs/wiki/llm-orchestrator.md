---
name: llm-orchestrator
description: Two-tier call layer for all model traffic — kind routing (local Ollama / cloud Anthropic-or-router), persisted monthly spend meter with hard ceiling, circuit-breaker degraded mode, bounded-queue backpressure
kind: component
sources:
  - internal/llm/llm.go
  - internal/llm/config.go
  - internal/llm/meter.go
  - internal/llm/health.go
  - internal/llm/providers.go
verified_against: a49d615ec26d41ff14784f5a8f03f89d0e6c96f9
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
gatekeeper's console turns and digests, [[metatron]]) go **cloud**. The local tier
(`providers.go`) speaks OpenAI-compatible chat-completions over raw HTTP (Ollama at
`http://localhost:11434/v1`, default model `gemma4:12b-mlx` — the operator's
always-on local model); the cloud tier is provider-selectable
(`cloud.provider`, TASK-15): the default `anthropic` uses the official
`anthropic-sdk-go` against the Messages API (`claude-opus-4-8` default), with
`cache_control` on system blocks so stable prompts (souls, charters) bill at
cache-read rates on repeat calls; `openai_compat` reuses the chat-completions
caller for OpenAI-compatible routers (e.g. a LAN-local 9router), requires
`cloud.endpoint`, and pins `stream: false` because some routers stream by
default.

**Priority lanes**: conversations (`KindConversation`) ride a per-tier priority
queue the worker drains first — dialogue turns are interactive, while planner
thoughts tolerate staleness (the reflex grace covers them). The opposite
extreme is caller-flagged: `Request.BestEffort` calls (musings,
`KindMusing`, local) are refused with `ErrTierBusy` the moment either
local queue has work waiting — flavor yields to real cognition. The flag
belongs to the caller so the mind can drop it as a fairness floor when a
musing has been starved too long (TASK-21, [[agent-mind]]). A worker-side hard cap
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
rejection guards only the high side). Estimators start from bootstrap seeds
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
`scriptworld new`; deleting the file disables the orchestrator entirely. Hosted-API
keys are never stored — only the *name* of an environment variable (`api_key_env`,
default `ANTHROPIC_API_KEY`). The one exception is the optional inline `api_key`
(both tiers), intended solely for keys that guard LAN-local routers; when both are
set the inline key wins. Provider values are validated at load time (`LoadConfig`
rejects unknown providers and `openai_compat` without an endpoint).

## Connections

[[daemon-lifecycle]] starts it when config exists; [[ipc-server]] exposes `llm_call`
and folds `StatusSnapshot` into the protocol status; [[cli-scriptworld]]'s `llm`
subcommand is the one-shot proof path; the [[tui-client]] metatron pane displays tier
health and spend; the meter persists via [[event-log]]'s store (meta table). TASK-7
(agent minds), TASK-9 (consolidation), TASK-11 (narrator), and TASK-12 (Metatron)
are the intended callers.

## Operational notes

Tested against httptest mock providers for both tiers: routing, cost math, ceiling
refusal with zero HTTP, circuit open/fast-fail/recovery, queue overflow, meter
persistence, and world-keeps-ticking-with-dead-endpoints. Live-verified against a
real Ollama instance end-to-end through the daemon. Budget reality check: nightly
consolidation volume at v1 scale ≈ $34/month on the default cloud model — inside the
$100 ceiling from the grounding session.
