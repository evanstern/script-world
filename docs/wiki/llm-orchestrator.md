---
name: llm-orchestrator
description: Provider-registry call layer for all model traffic — declared providers with ordered per-kind routing chains (spec 024), per-provider workers/breaker/queues/estimator, chain-walk fallback with recorded skips, one global spend ceiling with per-provider attribution, advisory endpoint leases
kind: component
sources:
  - internal/llm/llm.go
  - internal/llm/config.go
  - internal/llm/meter.go
  - internal/llm/health.go
  - internal/llm/providers.go
  - internal/llm/lease.go
  - internal/llm/pending.go
verified_against: 6eb8b60ceb65d760408051eadf50a789603efa18
---

# LLM orchestrator

`internal/llm` (TASK-6; generalized to a provider registry by spec 024 / TASK-35,
doctrine decision-5) is the single gateway for all model traffic. It lives entirely
**outside** the deterministic sim loop: LLM results reach the world only as recorded
inputs (TASK-7's job), so replay never re-calls a model — the determinism contract of
the substrate is structurally untouchable by inference.

## How it works

**Providers and routes** (`llm.go`, `config.go`, spec 024): model sources are
**declared** as a named registry (`Config.Providers`) and every call `Kind` maps to an
**ordered chain** of provider names (`Config.Routes`). Chain order is the operator's
complete placement ruling — membership means "meets this kind's quality floor",
position means preference; no runtime scoring, no model ever chooses a model
(decision-5 extends decision-4's deterministic-routing doctrine one level down). Each
provider declares a transport (`openai_compat` — Ollama, LAN routers, 9router — or
`anthropic`, the official SDK), endpoint, model, pricing, `parallel`, per-provider
`reasoning_effort`/`tool_mode`, and optional `endpoint_capacity`. "Tier" retired as a
routing concept; the surviving local-vs-cloud distinction is **pricing class**
(`provider.priced()`): zero-priced providers are never budget-refused and seed
local-class latency bootstraps. The legacy two-entry config (`local`/`cloud`) loads
forever via `deriveLegacy` — a two-provider registry named `local`/`cloud` with the
pre-024 routes (planner/conversation/meeting → local; consolidation/narrator/drama/
metatron → cloud), byte-identical behavior; declaring both shapes in one file is a
load error. `KindMusing` retired with spec 017: musing is a roster tool inside the
planner loop ([[agent-mind]], [[tool-loop]]).

**Chain-walk admission** (`Submit`, spec 024 US3): submission walks the kind's chain
in order and dispatches to the first admissible candidate; a candidate is skipped only
for a mechanical, observable reason — `wallet-exhausted` (priced candidates when the
ceiling is hit), `circuit-open`, `busy` (best-effort only), `queue-full` — recorded in
order on `Response.Skipped` (`[]RouteSkip{Provider, Reason}`). All candidates
inadmissible → the CHAIN HEAD's refusal error (`refusalFor`: the same
`ErrBudgetExhausted`/`ErrTierDown`/`ErrTierBusy`/`ErrQueueFull` sentinels as ever —
single-entry chains behave byte-identically to pre-024). Once a provider accepts a
job its failure is final: never re-dispatched elsewhere. A route may declare
`no_fallback` (single-entry chain enforced at load); `Request.Provider` pins a call to
a named provider, bypassing the walk while honoring that provider's admission
(`ErrUnknownProvider` guards a bad name). Two continuity pins ride this field:
a conversation SCENE resolves its provider once at scene start
([[social-fabric]]), and a tool-loop RUN pins at run start — including across the
spec-025 retry — via `ResolveProvider` ([[tool-loop]]); a persona never switches
voices mid-dialogue, a thought never switches models mid-transcript.
`Response.Provider` always names the serving provider.

**Transports** (`providers.go`): `openai_compat` speaks chat-completions over raw
HTTP, pins `stream: false` (some routers stream by default), and carries
`max_tokens` (from `Request.MaxTokens`, when positive) plus the provider's resolved
`reasoning_effort` (TASK-37: thinking-default models like gemma4 otherwise free-run
hidden chain-of-thought — live diagnosis measured 2–6 s calls inflated to 60–120 s);
`resolveReasoningEffort` keeps the nil/"" convention — zero-priced absent defaults
`"none"`, priced absent (and explicit `""` anywhere) sends nothing. `anthropic` uses
`anthropic-sdk-go` against the Messages API with `cache_control` on system blocks so
stable prompts (souls, charters) bill at cache-read rates. `newProviderCaller` builds
the right caller per declared transport. (A TASK-58 `ResponseSchema` structured-output
path was deleted as dead code in TASK-71; git history has it.)

**Agent tool-use loop transport** (`llm.go`/`providers.go`, TASK-52, spec 017; every
field additive — a request that sets none marshals byte-identical to before):
`Request.Tools` (`[]ToolDecl{Name, Description, InputSchema}`) declares the round's
tools; `Request.Turns` (`[]Turn{Role, Blocks}`, a `Block` one of text /
`ToolUseBlock` / `ToolResultBlock`) is the ephemeral multi-turn transcript replacing
`Prompt` when non-nil (never persisted, never replayed); `Request.SkipObserve` marks a
loop-internal per-round `Submit` so the worker feeds no fractional per-call sample to
the estimator. `Response.ToolCalls` carries emitted calls in order;
`Response.Stop` (`end_turn`/`tool_use`/`max_tokens`/`other`) is the mapped stop reason
[[tool-loop]]'s driver reads. The Anthropic caller sends native `ToolUnionParam`s and
`tool_use`/`tool_result` blocks (`anthropicInputSchema` round-trips schema keywords
the SDK's typed struct would drop, via `ExtraFields`); `openaiCompat.call` picks a
path per the provider's resolved `tool_mode`: `callNative` sends OpenAI-style
`tools`/`tool_calls`; `callJSON` is the FR-010 fallback for backends whose native
function calling is unreliable — tool catalog appended to the system prompt, every
reply grammar-constrained to a flat `{"tool", "args", "say"}` envelope, per-round call
IDs synthesized (`"env-<round>"` from the assistant-turn count) — a fallback-mode
transcript must keep exactly one assistant turn per round or synthesized IDs collide.

**Tool-call strategy and kind-scoped knobs** (`config.go`): `tool_mode` is
**per-provider** (`ProviderConfig.ToolMode`; legacy `local.tool_mode`/
`cloud.tool_mode` map onto the derived providers), normalized warn-not-error by
`resolveToolMode`, honored only by the `openai_compat` transport — the Anthropic path
is always native. Measured live (TASK-52 T027): cogito:3b never function-calls
natively (88/88 unusable) — its provider entry needs `"json"` wherever a tool-loop
kind can resolve to it. The kind-scoped knobs stay TOP-LEVEL (a property of the
thought class, never the provider — spec 024 R9): `Config.LoopMaxRounds`
(`Rounds()`: absent/0 → 8, clamp 1–16, warn-not-error) and `Config.MaxTokens`
(`*TokenBudgets`, spec 025 / TASK-72) — three per-kind response budgets, `planner`
(default 512), `metatron_turn` (1024), `consolidation` (1024), each normalized by
`PlannerTokens()`/`MetatronTurnTokens()`/`ConsolidationTokens()` (absent/0 → default,
1–4096 verbatim, clamp with warning; a POINTER so `omitempty` genuinely suppresses
the object and pre-025 configs round-trip byte-for-byte — preserved by the
shape-aware v2 `Config.MarshalJSON`). The daemon resolves all three at boot and
threads them into `mind.New`/`metatron.New`; conversation (128/224), meeting (72),
narrator (800), and metatron digest (400) budgets are deliberately NOT governed by
these knobs.

**Whole-loop latency feed** (`ObserveCognition(kind, provider, totalMillis)`,
TASK-52/spec 024): the tool-use loop's per-round `Submit`s each ride `SkipObserve`;
the loop reports exactly one whole-cognition wall time, attributed to the named
serving provider (its run pin — empty falls back to the chain head). Both feeding
paths share `feedEstimate`, normalizing by the kind's registered point cost and
firing the same per-provider recalibrate hook; [[tool-loop]]'s `Run` calls this only
on a completed termination (landed / model_done / cap_exhausted), never on the
failure family — mirroring the worker's successes-only doctrine below.

**Concurrency** (TASK-45, per-provider since spec 024): each provider owns `slots`
worker goroutines — N identical copies of one worker loop draining its two channels —
from its `parallel` via `Workers()` (absent/0 → 1, clamp 1–16 `maxLocalWorkers`,
warn-not-error; the daemon prints clamp warnings and the world always boots). An
`atomic.Int32` `inflight` per provider (incremented at dequeue, decremented on every
reply path) drives slot-aware best-effort admission.

**Priority lanes** (per-provider): conversations (`KindConversation`) ride a priority
queue idle workers drain first — dialogue is interactive, planner thoughts tolerate
staleness. The opposite extreme is caller-flagged: `Request.BestEffort` calls are
refused (`ErrTierBusy` / skip reason `busy`) when the candidate has queued work or no
idle slot — flavor yields to real cognition (`meeting.go`'s proposal rephrasing is
the current user; the caller-owned fairness-floor doctrine stands for any future
drop-when-busy kind). A worker-side hard cap (`workerCallCap`, 2 min) bounds any
single provider call so a hung transport can never wedge a provider. **Submit** is
synchronous with immediate admission control — that backpressure surface is what lets
local throughput cap effective sim speed. Bounded queues stay 32 per lane per
provider.

**Spend: one wallet, per-provider attribution** (`meter.go`, spec 024 US4): a single
global `monthly_budget_usd` ceiling, checked at admission per priced candidate BEFORE
any HTTP. Cost uses the serving provider's declared pricing; `Add(provider, cost)`
writes the unchanged total key `llm_spend_YYYY-MM` AND `llm_spend_YYYY-MM:<provider>`
under one lock — restarts never forget money, per-provider rows sum to the total, and
pre-024 months surface their remainder as unattributed. Zero-priced providers are
never budget-refused (pricing class, not tier identity — the one deliberate
behavioral edge vs pre-024: a hypothetical zero-priced cloud router now serves past
the ceiling).

**Degraded mode** (`health.go`, per-provider): a circuit breaker — 3 consecutive
failures open it (15 s backoff doubling to 5 min), an open circuit refuses instantly
(and is skipped by the chain-walk), one half-open probe tests recovery. Busy is not
down (TASK-22): the worker skips queued jobs whose caller already gave up and never
counts a failure when the caller's own ctx died mid-call — only genuine provider
failures and the worker cap strike the breaker. A killed model degrades the AI layer;
the daemon and loop never notice.

**Latency estimation** (TASK-32, [[cognition]], per-provider): each provider carries
a live `cognition.Estimator` of seconds-per-point — the worker samples each
*successful* call's wall time normalized by the kind's point cost; with `parallel` >
1 samples include server-side contention, converging on true concurrent-rate cost.
Estimators seed from `cognition.SeedFor(profile, name, zeroPriced)` — profile keyed
by provider name (legacy worlds' derived `local`/`cloud` keep matching), miss falls
back by pricing class; `SeedCalibration` re-seeds all providers at daemon start. The
mind-facing exports (spec 024): `EstimateForKind(kind)` returns the kind's CURRENT
ADMISSIBLE chain head's name + estimate (`admissibleHead`, a non-mutating read —
falls back to the chain head when none admissible), `ResolveProvider(kind)` is the
pin-resolution dry walk, `ProviderNames()`/`ProviderConfig(name)` serve calibrate,
and `Kinds()` still feeds the cognition registry's completeness gate at daemon start.
Per-provider recalibrate hooks fire once per breach episode via `SetRecalibrateHook`;
the mind records `cog.recalibration_recommended` (the provider name rides the
recorded payload's `Tier` field, kept for replay-schema stability).

**Advisory endpoint leases** (`lease.go`, spec 024 US5 — closes TASK-24): a provider
declaring `endpoint_capacity` C joins a cross-process lease pool keyed by its
normalized endpoint (lowercased scheme+host, default ports and trailing slash
stripped; sha256[:16] names the pool dir under `~/.promptworld/endpoint-leases/`).
Acquisition is a non-blocking `syscall.Flock(LOCK_EX|LOCK_NB)` sweep over slot files
`slot-00…slot-(C-1)` with jittered ~100 ms retries, in the worker AFTER the
stale-skip check, inside the 2-min call cap, BEFORE the provider call — so combined
in-flight calls across all participating worlds never exceed C, and the TASK-24
mutual breaker-thrash cannot recur. Crash-safe by construction (the kernel frees a
dead process's flocks); lease waiting never strikes the breaker and the estimator
measures from post-acquisition start. A wait over 2 s sets the pool's `contended`
flag (cleared by a sub-2 s acquisition; the flag is POOL-scoped — endpoint congestion
is one truth shared by providers on that endpoint) and surfaces per provider in
status. Undeclared capacity = zero lease syscalls, exactly pre-024 behavior; a
missing home dir disables leases with a warning (warn-not-error).

**Pending-thought registry** (`pending.go`, spec 028 US1): a mutex-guarded
`pendingRegistry` inventories every accepted-but-unfinished job — the adaptive
throttle governor's debt signal. `Submit` `add`s an entry (keyed by a
monotonic id carried on the internal `job`) the instant a candidate accepts,
BEFORE the non-blocking channel send, so a worker that dequeues immediately
can always find it to stamp; the worker's `dispatch` stamps wall time at
dequeue (zero while still queued); a deferred `remove` on every terminal path
of `Submit` (reply, provider error, caller-abandoned ctx) drains the entry, so
the registry empties to zero once all work quiesces — a leaked entry would be
a bug. `Orchestrator.PendingCognition()` snapshots the registry (copy under
the lock, arithmetic outside it) into `[]PendingThought{Kind, Provider,
PredictedSec, ElapsedSec}`: `PredictedSec` is the job's class point cost ×
its provider's CURRENT live seconds-per-point estimate (recomputed at read
time, so it tracks the freshest estimator state including spike rejection),
`ElapsedSec` is wall time since dispatch (0 while queued). The daemon's
governor sampler ([[cognition]], [[daemon-lifecycle]]) is the sole consumer,
polling this every `GovernorCadence` to derive aggregate staleness debt; the
registry itself is orthogonal to routing/metering/breaker machinery and adds
no new call-admission behavior.

**Status** (`StatusSnapshot`, spec 024 US6): `Status{Providers []ProviderStatus,
Month, Spent, Budget}`, sorted by name — one shape for legacy and v2 worlds (legacy
shows rows `local`/`cloud`). `ProviderStatus{Name, Model, Endpoint, Up, Queue,
Inflight, Slots, Contended, SpentUSD}`.

**Config** (`config.go`): `llm.json` in the save directory, written v2 by
`promptworld new`; deleting the file disables the orchestrator entirely. Hosted keys
are never stored — only an env var NAME (`api_key_env`, default `ANTHROPIC_API_KEY`);
the optional inline `api_key` is for LAN-router keys only and wins when both are set.
`resolveRegistry` is the single validation authority (LoadConfig and `New` both call
it): boot ERRORS name the offender for a route to an undeclared provider, an accepted
kind with no route, an unknown kind key, a duplicate provider in a chain, an empty
chain, `no_fallback` with chain length > 1, missing transport/model, `openai_compat`
without endpoint, or both config shapes at once; tuning knobs clamp with warnings,
never errors. `RouteConfig.UnmarshalJSON` accepts the bare-array shorthand
(`["a","b"]`) and the `{chain, no_fallback}` object; `MarshalJSON` re-emits the
shorthand, and the shape-aware `Config.MarshalJSON` round-trips both shapes —
including top-level `max_tokens` — byte-for-byte.

## Connections

[[daemon-lifecycle]] starts it when config exists; [[ipc-server]] exposes `llm_call`
and folds `StatusSnapshot` into the protocol status; [[cli-promptworld]]'s `llm`
subcommand is the one-shot proof path (naming the serving provider and any skips) and
its `calibrate` iterates declared providers; the [[tui-client]] metatron pane renders
the provider table and spend; the meter persists via [[event-log]]'s store (meta
table). TASK-7 (agent minds), TASK-9 (consolidation), TASK-11 (narrator), and TASK-12
(Metatron) are the callers. [[tool-loop]] is the transport's other consumer (spec
017): it drives `Request.Tools`/`Turns`/`SkipObserve` and
`Response.ToolCalls`/`Stop` through `Submit`, pins its run's provider via
`ResolveProvider`, and reports whole-cognition latency via `ObserveCognition` — used
by both [[agent-mind]]'s `runPlan` and [[metatron]]'s `Turn`. [[social-fabric]]'s
conversation scenes pin per scene through the same `Request.Provider` field.
[[daemon-lifecycle]]'s governor sampler polls `PendingCognition` every
`GovernorCadence` and feeds it to [[cognition]]'s `Debt`/`Governor`.

## Operational notes

Tested against httptest mock providers: legacy-equivalence (the standing regression
suite — a legacy config's routing/refusals/metering/status pinned byte-identical),
the boot validation matrix, chain-walk skips per reason, pin admission,
no-redispatch, per-provider estimator attribution under concurrent two-provider load,
meter attribution summing (Σ providers + unattributed == total, across store
reopens), lease pools bounding combined in-flight across two orchestrators with
crash reclaim — all under `go test -race`. Live-verified (TASK-35 T019, real
Ollama): conversation → cogito:3b, planner → gemma4:12b-mlx concurrently loaded;
a dead provider's breaker opened after 3 hard failures and the next call recorded
`skipped: bogus (circuit-open)` — post-dispatch failure is final, exactly as
designed. Motivating measurements: one worker serialized everything (TASK-45: 130 s
queue waits behind 19 s calls) while the server ran 4 concurrent cogito calls in
0.98 s wall; 48–128-token structured outputs are 3B-viable while prose is not
(TASK-35 notes) — the division of labor the registry exists to express. Budget
reality check: nightly consolidation ≈ $34/month on the default cloud model, inside
the $100 ceiling.
