# Feature Specification: Multi-Provider Routing — Registry and Ordered Chains

**Feature Branch**: `task-35-provider-routing`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "Multi-provider LLM division of labor (TASK-35): evolve the
two-tier kind→tier table in internal/llm into a declared provider registry + ordered
per-kind routing chains, per decision-5."

**Doctrine**: decision-5 (provider division of labor). Parent doctrine: decision-4 (the
cognition horizon — deterministically routed, never model-judged). This spec extends that
rule one level down: no model, and no runtime heuristic, ever chooses a model. Providers
are declared, chains are ordered by the operator, and every skip has a mechanical,
observable reason.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Providers are declared, routes are chains, yesterday's worlds still boot (Priority: P1) 🎯 MVP

As an operator, I declare my model endpoints once as a named provider registry in the
world's model configuration, and say per call kind which providers may serve it and in
what order — while every existing world with the old two-entry (local/cloud) config keeps
working without me touching anything.

**Why this priority**: everything else in this feature hangs off the registry and routes
existing. The equivalence guarantee (legacy config → two providers + today's routes,
identical behavior) makes this slice shippable alone: it is a pure generalization with
zero behavior change for existing worlds.

**Independent Test**: boot a world with an untouched legacy config and observe identical
routing, admission errors, spend metering, and status; boot a world with a v2 registry
config and observe each kind served by its chain head; boot with an invalid config (route
to an unknown provider, kind with no route) and observe a clear startup failure.

**Acceptance Scenarios**:

1. **Given** a world whose model config is the legacy local/cloud shape, **When** the
   daemon starts and traffic flows, **Then** every call kind is served exactly as today
   (same endpoints, same admission errors, same metering) with no config edit required.
2. **Given** a v2 config declaring named providers and a routes table, **When** a call of
   kind K is submitted, **Then** it is dispatched to the first provider in K's chain and
   the response names that provider.
3. **Given** a v2 config whose routes name an unknown provider, or omit a call kind the
   orchestrator accepts, **When** the daemon starts, **Then** startup fails with an error
   naming the offending route — never a runtime surprise.
4. **Given** a provider entry with out-of-range tuning (parallel above the cap, unknown
   tool mode), **When** the daemon starts, **Then** the value clamps with an operator
   warning and the world boots (existing warn-not-error doctrine, per provider).

---

### User Story 2 - Division of labor: cheap chatty kinds on the small model, prose on the big one (Priority: P2)

As an operator running two always-loaded local models (a ~1 s small model that serves 4
concurrent calls, and a ~20 s quality model), I route high-volume structured kinds
(conversation turns) to the small parallel model and keep prose kinds (planner,
narrator-adjacent work) on the quality model — by editing configuration only — and the
sim's latency-aware machinery sees each provider's true speed separately.

**Why this priority**: this is the measured win that motivated the feature (4 concurrent
small-model calls in 0.98 s wall vs 20 s quality-model calls under load). It needs US1's
registry but nothing else.

**Independent Test**: declare two local providers with different `parallel` values, route
conversation to one and planner to the other, run traffic, and verify per-provider worker
counts, per-provider latency estimates, and that responses name the expected provider per
kind.

**Acceptance Scenarios**:

1. **Given** two declared providers on one host and routes splitting kinds between them,
   **When** mixed traffic runs, **Then** each kind's calls land on its chain head and
   concurrency per provider matches that provider's declared `parallel`.
2. **Given** per-provider traffic with different real latencies, **When** the
   latency-aware layer (cognition horizon) asks "how long will a thought of kind K
   take?", **Then** the answer comes from K's chain head's own live estimate — a fast
   small model is never averaged with a slow quality model.
3. **Given** the whole-cognition observation path (tool-use loop), **When** a loop
   completes on provider P, **Then** exactly P's estimator receives the observation.

---

### User Story 3 - Fallback is chain-walking; personas never switch voices (Priority: P3)

As an operator, when a preferred provider is mechanically unavailable (circuit open,
wallet empty for a priced provider, queue full), calls of that kind flow to the next
provider in the chain with the skip reason recorded — but a conversation scene that
started on one provider keeps that provider for every turn, and kinds I mark no-fallback
fail rather than substitute.

**Why this priority**: resilience and persona integrity. Depends on US1 (chains) and is
most valuable once US2 spreads kinds across providers.

**Independent Test**: force each skip condition against a chain of two providers and
observe dispatch to the second with the reason recorded; kill a pinned scene's provider
mid-scene and observe failure-to-tolerance (never a provider switch); mark a route
no-fallback and observe head-failure surfaces as an error.

**Acceptance Scenarios**:

1. **Given** kind K routed [A, B] and A's circuit open, **When** K is submitted, **Then**
   the call dispatches to B and the routing record says A was skipped circuit-open.
2. **Given** kind K routed [A, B] where A is priced and the monthly wallet is exhausted,
   **When** K is submitted, **Then** the call dispatches to B (if B is zero-priced or
   affordable) and the skip reason is wallet-exhausted.
3. **Given** all candidates of K inadmissible, **When** K is submitted, **Then** the call
   fails fast with the chain head's refusal reason — no queueing, no retry loop — and the
   caller's existing degrade path (reflex layer / tolerance machinery) proceeds as today.
4. **Given** a conversation scene whose first turn resolved to provider A, **When** A
   becomes inadmissible mid-scene, **Then** subsequent turns of that scene fail into the
   conversation tolerance machinery — they are never re-routed to B.
5. **Given** a route marked no-fallback, **When** its head is inadmissible, **Then** the
   call fails with that reason even if other providers exist.
6. **Given** a provider accepted a job and then errored mid-call, **When** the error
   surfaces, **Then** the failure is that provider's failure (breaker strike per today's
   rules) and the job is NOT re-dispatched elsewhere.

---

### User Story 4 - One wallet, per-provider attribution (Priority: P4)

As an operator, I keep a single monthly spend ceiling for the whole world, every call is
priced by its provider's declared rates, and I can see how the month's spend divides
across providers. Zero-priced providers are never refused for budget reasons.

**Why this priority**: money correctness. The global ceiling already exists; this story
generalizes pricing to N providers and adds attribution.

**Independent Test**: run priced traffic across two priced providers plus a zero-priced
one; verify per-provider attribution sums to the meter total, ceiling refusal applies
only to priced candidates, and attribution survives a daemon restart.

**Acceptance Scenarios**:

1. **Given** calls served by providers with different pricing, **When** the month
   accumulates, **Then** each call's cost uses its provider's rates and per-provider
   totals sum to the global meter total.
2. **Given** the ceiling reached, **When** a kind whose chain contains a zero-priced
   provider is submitted, **Then** it is served by that provider; a kind whose chain is
   all-priced is refused with the budget error.
3. **Given** a daemon restart mid-month, **When** status is read, **Then** per-provider
   attribution and the global total are unchanged (persisted, as today).

---

### User Story 5 - Worlds sharing an endpoint coordinate instead of thrashing (Priority: P5)

As an operator running two worlds against one local endpoint, I declare the endpoint's
true concurrent capacity once per provider; worlds acquire advisory slots through the
operating system before dispatching, so combined load can no longer push calls past the
worker cap and trip both circuit breakers — and when a world is waiting on a contended
endpoint, its status says "contended" plainly.

**Why this priority**: closes TASK-24's observed failure (mutual circuit-thrash). Needs
only US1. Advisory and crash-safe: a crashed world's slots free automatically.

**Independent Test**: two daemons, one endpoint, declared capacity; drive both at
saturation and verify neither breaker opens from contention alone, total in-flight
against the endpoint never exceeds capacity, and the waiting world's status shows
contended; kill one daemon holding slots and verify the other acquires them.

**Acceptance Scenarios**:

1. **Given** two worlds with the same endpoint declared at capacity C, **When** both are
   saturated, **Then** combined in-flight calls against the endpoint never exceed C and
   neither world's breaker opens due to contention-induced timeouts.
2. **Given** a world waiting longer than a threshold for an endpoint slot, **When**
   status is read, **Then** the provider reports contended (and stops reporting it once
   flow resumes).
3. **Given** a world holding slots crashes, **When** the surviving world needs capacity,
   **Then** the dead world's slots are reclaimed without operator action.
4. **Given** a provider with no declared endpoint capacity, **When** traffic runs,
   **Then** behavior is exactly as without the lease mechanism (opt-in feature).

---

### User Story 6 - The operator can see where every call went and why (Priority: P6)

As an operator, the status surface and TUI show a per-provider table — model, endpoint,
up/down, queue depth, in-flight vs slots, contended, month spend share — and recent
routing decisions are legible: which provider served a call, and which candidates were
skipped for which mechanical reasons.

**Why this priority**: legibility completes the feature but every underlying fact is
already produced by US1–US5; this story is surfacing.

**Independent Test**: run mixed traffic with a forced fallback and read the status
payload and TUI: per-provider rows are present and accurate, and the fallback shows its
skip reason.

**Acceptance Scenarios**:

1. **Given** a running world with a v2 registry, **When** status is read (protocol and
   TUI), **Then** every declared provider appears with model, endpoint, health, queue
   depth, in-flight/slots, contended flag, and spend attribution.
2. **Given** a call served by a fallback candidate, **When** its routing record is
   inspected, **Then** it names the serving provider and each skipped candidate with its
   mechanical reason.
3. **Given** a legacy-config world, **When** status is read, **Then** the same
   per-provider table appears (two derived providers) — one status shape for all worlds.

---

### Edge Cases

- A route lists the same provider twice → load-time validation error (a chain is a set
  with order).
- Two provider entries share endpoint+model under different names → allowed (distinct
  tuning profiles are legitimate); endpoint-capacity leases key on the normalized
  endpoint, so shared-endpoint entries share one capacity pool.
- Best-effort (drop-when-busy) calls: chain-walking applies, but a candidate is
  best-effort-admissible only with an idle slot and empty queues (today's semantics, per
  provider); if no candidate qualifies, refuse with the busy error as today.
- The priority lane (interactive conversation turns jump planner backlog) exists per
  provider; a scene pinned to provider A rides A's priority lane only.
- Wallet exhausted mid-scene for a pinned priced provider → the pin holds; turns are
  refused with the budget error into the tolerance machinery (persona integrity outranks
  completion).
- Calibration profiles recorded under legacy tier names seed the two derived legacy
  providers; a v2 world seeds each provider from its own profile entry when present,
  otherwise from the bootstrap default for its pricing class.
- Config is read at boot only (as today): route/registry edits take effect on daemon
  restart; there is no hot-reload in scope.
- Endpoint lease acquisition must never block the sim loop: waiting happens in the
  orchestrator's dispatch path (outside the deterministic loop), bounded by the worker
  call cap.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The model configuration MUST accept a provider registry: uniquely named
  entries each declaring transport family (OpenAI-compatible or Anthropic), endpoint,
  model, input/output pricing, concurrency (`parallel`), reasoning-effort posture, and
  tool-call strategy — the same knob set the two fixed tiers expose today, now per entry.
- **FR-002**: The model configuration MUST accept a routes table mapping every call kind
  the orchestrator accepts to a non-empty ordered chain of declared provider names; a
  route MAY be marked no-fallback. Chain order is the operator's complete quality/cost
  ruling — the system MUST NOT reorder, score, or infer placement at runtime.
- **FR-003**: Configuration loading MUST fail at startup — with an error naming the
  problem — for: a route naming an undeclared provider, a duplicate provider name, a
  chain listing a provider twice, an accepted call kind with no route, or a v2 config
  missing required provider fields. Tuning knobs (parallel, tool mode, reasoning effort)
  MUST keep the warn-and-clamp posture, per provider.
- **FR-004**: The legacy two-entry (local/cloud) configuration shape MUST continue to
  load with zero edits, deriving a two-provider registry and a routes table identical to
  today's kind→tier mapping, with behavior indistinguishable from today (routing,
  admission errors, metering, status content, calibration seeding).
- **FR-005**: Each declared provider MUST own a full instance of today's per-tier
  machinery with unchanged semantics: bounded queue + interactive priority lane, N worker
  slots per its `parallel`, circuit breaker with identical strike/backoff/probe rules,
  and a live latency estimator.
- **FR-006**: Admission MUST walk the kind's chain in order and dispatch to the first
  admissible candidate. A candidate is skipped only for a mechanical, observable reason:
  circuit open; wallet exhausted (priced candidates only); queue full. Best-effort
  requests additionally require an idle slot and empty queues on the candidate.
- **FR-007**: When every candidate is inadmissible, the submission MUST fail immediately
  with the chain head's refusal reason; existing caller degrade paths (reflex layer,
  conversation tolerance) apply unchanged. After a provider accepts a job, its failure is
  final for that call — the orchestrator MUST NOT re-dispatch a failed call to another
  provider.
- **FR-008**: A conversation scene MUST resolve its provider once at scene start and use
  it for every turn of that scene; mid-scene inadmissibility or failure surfaces as
  failure to the existing tolerance machinery. A scene MUST never change providers.
- **FR-009**: A single global monthly spend ceiling MUST govern all priced traffic;
  refusal happens at admission (before any network call) exactly as today. Zero-priced
  providers MUST never be budget-refused. Per-provider budgets are explicitly out of
  scope (decision-5).
- **FR-010**: Every call's cost MUST be computed from its serving provider's declared
  pricing, and monthly spend MUST be attributed per provider, persisted such that
  restarts lose nothing; per-provider attribution MUST sum to the global meter total.
- **FR-011**: Every response MUST name its serving provider (in addition to model), and
  every routing decision MUST record, for telemetry/status consumption, the serving
  provider plus any skipped candidates with their skip reasons.
- **FR-012**: The status surface MUST present a per-provider table — name, model,
  endpoint, health (up/down), queue depth, in-flight vs slots, contended flag, month
  spend attribution — for v2 and legacy worlds alike (legacy shows its two derived
  providers), and the TUI MUST render it.
- **FR-013**: The latency-prediction seam read by the cognition horizon MUST become
  provider-granular: asking about a kind returns the live estimate of the kind's current
  admissible chain head, deterministically; per-call and whole-cognition observations
  MUST feed exactly the serving provider's estimator, preserving the existing
  successes-only and skip-observe rules.
- **FR-014**: Calibration seeding MUST key by provider name, with legacy tier-keyed
  profiles seeding the derived legacy providers, and providers absent from a profile
  seeding from pricing-class bootstrap defaults; the recalibration drift hook fires per
  provider.
- **FR-015**: A provider MAY declare an endpoint concurrent-capacity; when declared,
  dispatch MUST acquire an advisory cross-process slot (operating-system level,
  crash-reclaimable, keyed by normalized endpoint) before calling, bounding combined
  in-flight calls across all worlds honoring the declaration. Undeclared capacity MUST
  mean exactly today's behavior. Lease waiting MUST NOT strike the circuit breaker and
  MUST be bounded by the existing worker call cap.
- **FR-016**: A provider waiting on a contended endpoint beyond a threshold MUST report
  contended in status until flow resumes.
- **FR-017**: New-world scaffolding MUST write the v2 registry shape with defaults
  semantically identical to today's two-tier defaults.
- **FR-018**: Determinism and replay MUST be untouched: all routing, leasing, and
  metering happen outside the deterministic sim loop; model output reaches the world
  only as recorded events, and no recorded event's meaning depends on which provider
  produced it.

### Key Entities

- **Provider**: a named, declared model source — transport family, endpoint, model,
  pricing, concurrency, tuning posture — owning private machinery (queue, lane, workers,
  breaker, estimator) and monthly spend attribution.
- **Route**: one call kind's ordered chain of provider names plus a no-fallback flag; the
  operator's complete placement ruling for that kind.
- **Routing decision**: the record of one submission — serving provider and ordered skip
  reasons — consumed by telemetry and status.
- **Wallet**: the single global monthly ceiling plus per-provider spend attribution,
  persisted monthly.
- **Endpoint lease**: an advisory cross-process concurrency slot on a normalized
  endpoint, held only while a call is in flight, reclaimed on process death.
- **Provider status row**: the operator-facing snapshot of one provider (health, queue,
  slots, contended, spend).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of existing worlds (legacy config files) boot and run with zero config
  edits and no observable behavior change in routing, refusals, spend, or replay.
- **SC-002**: An operator can move a call kind's traffic to a different declared provider
  by editing configuration only (no code change) and observe, within one daemon restart,
  every call of that kind served by the new provider.
- **SC-003**: With conversation kinds routed to a small parallel provider per the
  measured division of labor, conversation turns dispatch concurrently up to that
  provider's declared parallelism while prose kinds continue on the quality provider —
  under mixed load, neither starves the other's queue.
- **SC-004**: When a chain's preferred provider goes down, the very next submission of
  that kind is served by the fallback candidate (no thrash window), and the skip reason
  is visible in status/telemetry; on recovery, traffic returns to the preferred provider
  on the next submission after the breaker's normal probe succeeds.
- **SC-005**: Zero conversation scenes ever change provider mid-scene, across all tests
  and live runs.
- **SC-006**: Two saturated worlds sharing one declared-capacity endpoint run without
  either circuit breaker opening from contention, where the same load without the
  declaration reproduces TASK-24's mutual thrash; the waiting world's contended state is
  visible in its status.
- **SC-007**: Per-provider spend attribution sums exactly to the global meter total in
  every test, including across restarts.
- **SC-008**: Every invalid routes/registry configuration in the validation matrix fails
  at boot with an error naming the offending entry; none produce a runtime surprise.

## Assumptions

- Configuration remains boot-time-only (as today); hot reload of the registry/routes is
  out of scope.
- The status protocol shape may change in this feature; daemon and TUI ship together in
  this repo, so the per-provider status table replaces the fixed local/cloud pair as the
  one status shape (legacy worlds simply show two rows).
- Per-provider concurrency keeps the existing clamp bounds (1–16 workers, queue cap 32)
  unless the operator's declaration says less.
- "Tier" retires as a routing concept; pricing (zero vs nonzero) is the only remaining
  local-vs-cloud distinction, used for budget refusal and estimator bootstrap seeds.
- The endpoint-lease mechanism is advisory: only worlds that declare a capacity
  participate; it protects cooperating worlds, not adversarial processes.
- TASK-42's conversation tolerance machinery (shipped) is the required companion for
  routing chatty kinds to small models; this spec does not change it.
- Existing per-kind point costs and staleness budgets (cognition registry) are unchanged;
  this feature changes who answers, never what may be asked.
