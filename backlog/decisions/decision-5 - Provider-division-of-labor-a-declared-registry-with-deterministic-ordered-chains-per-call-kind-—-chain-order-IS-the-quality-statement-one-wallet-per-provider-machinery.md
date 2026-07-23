---
id: decision-5
title: >-
  Provider division of labor: a declared registry with deterministic ordered
  chains per call kind — chain order IS the quality statement; one wallet;
  per-provider machinery
date: '2026-07-23 15:17'
status: proposed
---
## Context

Multi-provider design session (TASK-35, 2026-07-23). TASK-6's orchestrator routes by a
fixed two-row table — kind → local | cloud (`internal/llm/llm.go:53`) — with exactly one
provider per tier. Reality has outgrown it: the operator runs gemma4:12b-mlx (quality
prose, ~20 s under load) AND cogito:3b (~1 s warm, natively parallel — 4 concurrent calls
in 0.98 s wall) on one Ollama, a LAN 9router as an OpenAI-compatible cloud alternative,
and Anthropic direct. Live measurements (TASK-35 notes, 2026-07-21) show a real division
of labor: 48–128-token structured outputs (conversation turns, tool-loop rounds) are
3B-viable while planner/narrator prose is not, and both models fit memory simultaneously.
Meanwhile TASK-24 observed two worlds sharing one endpoint thrashing each other's circuit
breakers, and TASK-42 measured that small models raise empty-utterance rates — quality
routing and failure tolerance are coupled. Everything per-tier today — breaker, bounded
queue + priority lane, N worker slots (TASK-45), seconds-per-point estimator (TASK-32) —
exists exactly twice, hard-wired.

## Decision

**Providers are declared, never discovered; routes are ordered chains, never judgments.**
llm.json grows a **provider registry** (named entries: transport `openai_compat` |
`anthropic`, endpoint, model, pricing, `parallel`, `reasoning_effort`, `tool_mode`) and a
**routes table**: call kind → ordered chain of provider names. This extends decision-4's
doctrine one level down: no model — and no heuristic — ever chooses a model.

- **Chain order IS the quality statement.** A kind's chain is the operator's complete,
  explicit ruling: membership means "meets this kind's quality floor", position means
  preference. Capability tags, scoring functions, and inferred placement are rejected —
  they add machinery whose verdicts an operator cannot predict. Cost, latency, and quality
  axes are inputs the OPERATOR weighs when writing the chain, not runtime variables.
- **Fallback is admission-time chain-walking.** Submit walks the kind's chain and gives
  the job to the first admissible provider; a candidate is skipped only for mechanical,
  observable reasons — circuit open, wallet empty (priced providers only), queue full.
  All candidates inadmissible → the call fails with the head's error; the deterministic
  layer (reflex) remains the universal degrade action per decision-4. No retry-elsewhere
  after dispatch: once a provider accepts a job, its failure is that provider's failure.
- **Persona continuity pins.** A conversation scene resolves its provider once at scene
  start and keeps it for every turn of that scene — a villager's voice never switches
  models mid-dialogue; mid-scene failure goes to TASK-42's tolerance machinery, not to a
  different model. A route may also be declared `no_fallback` (chain of one) for kinds
  where substitution is worse than silence.
- **One wallet.** A single global `monthly_budget_usd` ceiling stands; the meter prices
  every call by its provider's declared pricing and attributes spend per provider.
  Zero-priced providers are never budget-refused. Per-provider budgets are rejected: the
  operator has one wallet, and partitioning it re-creates the stranded-budget problem the
  ceiling was built to avoid.
- **The machinery multiplies; the semantics don't.** Breaker, bounded queue + priority
  lane, worker slots, and the seconds-per-point estimator become per-provider instances of
  today's exact per-tier semantics. `Tier` retires as a routing concept; "local vs cloud"
  survives only as pricing (zero-priced ≙ local-like). The cognition horizon reads the
  estimate of the kind's currently-admissible chain head — routing preview and latency
  suppression stay deterministic.
- **Endpoint capacity is registry truth; worlds sharing an endpoint coordinate through
  the OS.** `parallel` declares per-provider in-process concurrency (as TASK-45). For
  TASK-24's cross-world contention, providers may declare an endpoint capacity honored via
  advisory file leases (flock slot files keyed by normalized endpoint URL) — crash-safe,
  daemon-to-daemon-protocol-free; a world that cannot get a slot queues and reports
  **contended** in status instead of striking the breaker. This subsumes TASK-24's
  advisory-lock option.
- **Legibility is part of the route.** Every response names its provider; every routing
  decision that skipped a candidate records why (`fallback: gemma-local circuit-open`).
  Status/TUI grow a per-provider table: model, endpoint, up, queue, inflight/slots,
  contended, spend share.
- **Legacy llm.json keeps working.** The old `local`/`cloud` shape loads as a two-provider
  registry with routes equal to today's table — byte-identical behavior, no migration step.

## Consequences

- llm.json becomes the deployment's division-of-labor document: the operator can put
  conversation turns on cogito:3b (parallel, ~1 s) while planner/narrator prose stays on
  gemma/cloud — the measured sweet spot — by editing chains, no code change.
- Routing a chatty kind to a small model raises empty-utterance risk; TASK-42's
  tolerate-one-bad-utterance/outcome machinery is the required companion (shipped).
  Retune chains from telemetry, never auto-degrade quality floors at runtime.
- Validation gets stricter: a route naming an unknown provider, or an orchestrator kind
  with no route, is a boot-time failure (mirrors the cognition registry completeness gate).
- The mind's routing-preview seam changes shape (provider-granular estimators); the
  spec must keep `SecondsPerPoint`-equivalent reads deterministic and cheap.
- TASK-24 closes into this design's endpoint-lease story; its status-hint fallback
  ("model contended") ships as part of the lease wait surface.
- Replay and determinism are untouched: routing happens outside the sim loop; provider
  choice affects which model answered, never how recorded events replay.

Spec: specs/023-provider-routing (TASK-35).