---
name: llm-orchestrator
description: Two-tier call layer for all model traffic — kind routing (local Ollama / cloud Anthropic), persisted monthly spend meter with hard ceiling, circuit-breaker degraded mode, bounded-queue backpressure
kind: component
sources:
  - internal/llm/llm.go
  - internal/llm/config.go
  - internal/llm/meter.go
  - internal/llm/health.go
  - internal/llm/providers.go
verified_against: cee600e086a1be15868205c16c395ee33aaa397e
---

# LLM orchestrator

`internal/llm` (TASK-6) is the single gateway for all model traffic. It lives entirely
**outside** the deterministic sim loop: LLM results reach the world only as recorded
inputs (TASK-7's job), so replay never re-calls a model — the determinism contract of
the substrate is structurally untouchable by inference.

## How it works

**Tiers and routing** (`llm.go`): `Kind` → tier per the grounding decisions —
`planner` and `conversation` go **local** (free, the only viable home for ~3,800+
calls/day); `consolidation`, `narrator`, and `drama` go **cloud**. The local tier
(`providers.go`) speaks OpenAI-compatible chat-completions over raw HTTP (Ollama at
`http://localhost:11434/v1` by default); the cloud tier uses the official
`anthropic-sdk-go` against the Messages API (`claude-opus-4-8` default), with
`cache_control` on system blocks so stable prompts (souls, charters) bill at
cache-read rates on repeat calls.

**Submit** is synchronous with immediate admission control, each failure mode a
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
half-open probe tests recovery, success resets. A killed model degrades the AI layer;
the daemon and loop never notice.

**Config** (`config.go`): `llm.json` in the save directory, written with defaults by
`scriptworld new`; deleting the file disables the orchestrator entirely. API keys are
never stored — only the *name* of an environment variable (`api_key_env`, default
`ANTHROPIC_API_KEY`).

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
