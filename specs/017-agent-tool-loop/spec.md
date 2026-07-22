# Feature Specification: Agent Tool-Use Loop

**Feature Branch**: `017-agent-tool-loop`

**Created**: 2026-07-22

**Status**: Draft

**Input**: User description: "Agent tool-use loop: minds call tools instead of prompt stuffing (TASK-52, builds on spec 014 tool registry / TASK-53). An agentic loop for agent minds — a mind call can declare a set of tools from the registry; the model may respond with tool calls; the loop executes them and feeds results back until the model produces a final answer, with a hard iteration/budget cap."

## Core Principle (preserved verbatim from TASK-52 design decisions)

> A tool call is a REQUEST; an event is the FACT; the gate decides; the executor grounds
> work in time and space. Speaking/musing/thinking are tools too — game-state integrity
> applies to expression, not just world mutation.

This principle governs every requirement below. The model never mutates the world; it
requests. The existing landing doors (the intent ladder and the social whitelist) remain
the only paths by which a request becomes a fact.

## Clarifications

### Session 2026-07-22

- Q: Local-tier tool-calling strategy and fallback? → A: Native-first — the local
  transport's native function-calling API is tried first; the fallback is a
  provider-agnostic structured-output convention (schema-constrained JSON emulating a
  tool call), selected by per-model configuration. Cloud uses its native tool interface.
- Q: Does the separately-scheduled musing channel get removed in this task? → A: Yes —
  removed now. Musing happens only when a loop cognition chooses the muse tool; the
  cadence-fired best-effort musing path is deleted.
- Q: Which cognitions adopt the loop in this task? → A: Both the villager planner-class
  cognition and the metatron turn (its tools are already in the registry with the charge
  gate). Conversations and nightly consolidation stay on their current mechanics.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A mind acts by calling a tool (Priority: P1)

An agent's cognition fires (planner cadence or scene edge, as today). Instead of the
model replying with free text that the mind parses, the cognition presents the agent's
tool roster to the model. The model responds by calling exactly one acting tool (a world
verb like `forage`, or an expressive tool like `muse`). The loop hands the call to the
tool's handler, which routes it through the existing landing door; the gate verdict comes
back; the cognition ends with a recorded outcome. Before acting, the model may call
read-class tools (mid-loop lookups that return data and change nothing) and receive
their results fed back, until it commits to its one action — all within a hard iteration
cap.

**Why this priority**: this is the deliverable — the bounded execute-and-feed-back loop.
Everything else (replay safety, observability, metering) qualifies it. It is also the
prerequisite for TASK-16's journal tools, which are read/write tools with no home until
this loop exists.

**Independent Test**: run a cognition against a stub model that (a) calls one world tool
immediately, and (b) calls a read tool, then a world tool. Both produce a landed intent
and a recorded outcome; the loop terminates within the cap in all cases.

**Acceptance Scenarios**:

1. **Given** an agent whose cognition declares its roster, **When** the model responds
   with a single valid acting tool call, **Then** the call is dispatched through the
   existing landing door, the gate's verdict is recorded, and the cognition completes
   with the same outcome telemetry a landed/rejected thought produces today.
2. **Given** a roster containing read-class tools, **When** the model calls a read tool,
   **Then** the handler's data is fed back to the model as a tool result, no event is
   emitted for the lookup itself, and the loop continues.
3. **Given** a model that keeps calling read tools without acting, **When** the hard
   iteration cap is reached, **Then** the loop terminates, the cognition ends with a
   recorded failure outcome, and no partial world mutation has occurred.
4. **Given** a model that attempts a second acting tool call after one has already been
   accepted in the same cognition, **When** the second call arrives, **Then** it is
   rejected, the rejection is recorded as an artifact, and the loop ends.

---

### User Story 2 - Replay reproduces state without re-running any loop (Priority: P1)

An operator replays an event log (crash recovery, audit, or test). Every effect a tool
loop ever caused is already present as ordinary events; replay applies them through the
reducer and reaches byte-identical state. No model call, no tool handler, and no loop
iteration executes during replay. Histories written before this feature replay
byte-identically under the new code.

**Why this priority**: determinism is the project's non-negotiable invariant; a loop
that leaked non-determinism into replay would be worse than no loop.

**Independent Test**: run a simulation in which agents act via tool loops, snapshot,
replay from genesis and from the snapshot; final states are byte-identical to the live
run. Run the existing replay suite over pre-feature fixture logs; it passes unmodified.

**Acceptance Scenarios**:

1. **Given** a live run where cognitions used mid-loop read tools and landed acting
   calls, **When** the log is replayed, **Then** the resulting state is byte-identical
   to the live state and no tool handler was invoked.
2. **Given** an event log written before this feature, **When** it is replayed under the
   new code, **Then** every payload byte matches (additive fields absent from old
   events stay absent).

---

### User Story 3 - Every tool call is a first-class, correlatable artifact (Priority: P2)

An operator investigating "why did Ada start foraging at tick 40,000?" queries the event
log. They find the grounding event (`agent.intent_set`) carrying the job identifier of
the causing cognition, follow it to the recorded tool-call request, its gate verdict, and
any preceding read-tool lookups — the full chain `tool call → verdict → grounding` by
identifier alone, with no adjacency inference. Calls that were rejected or never grounded
are equally present.

**Why this priority**: board AC#5. Today the call itself has no independent record and
correlating a completion back to its causing thought requires agent+adjacency inference;
this story cures that. It depends on Story 1 existing but is separable work.

**Independent Test**: after a run, pick any intent-set event, extract its job identifier,
and mechanically retrieve the causing tool call and its verdict from the log. Pick a
rejected call and confirm it is present with its rejection reason despite grounding
nothing.

**Acceptance Scenarios**:

1. **Given** a landed acting tool call, **When** the grounding event is inspected,
   **Then** it carries the causing cognition's job identifier, and that identifier
   locates the recorded call and verdict without scanning neighboring events.
2. **Given** a tool call that the gate rejected (or that was malformed / off-roster),
   **When** the log is queried, **Then** the call and its verdict are present as
   recorded artifacts even though no grounding event exists.
3. **Given** events written before this feature (no job identifier on grounding
   payloads), **When** they are read, serialized, or replayed, **Then** they are
   byte-identical to before.

---

### User Story 4 - The loop works on both tiers, with a documented fallback (Priority: P2)

Cognitions run on whichever tier routes them today. On the cloud tier the loop uses the
provider's native tool-calling interface. On the local tier — where function-calling
quality varies by model — the loop works via the chosen local strategy, and for any tier
or model that cannot tool-call reliably there is one explicit, documented fallback
convention, so no tier is silently broken.

**Why this priority**: board AC#3. Without local-tier coverage the loop only serves the
few cloud-routed cognition kinds and the primary consumers (planner-class cognitions,
journal tools) never benefit.

**Independent Test**: run the same cognition against one local-tier model and the
cloud-tier provider; both complete Story 1's scenarios. Point the loop at a model known
to fail structured tool calls; the fallback engages and the cognition still terminates
with a recorded outcome (never a hang or an unrecorded drop).

**Acceptance Scenarios**:

1. **Given** the cloud tier, **When** a cognition runs the loop, **Then** tool calls are
   exchanged via the provider's native tool interface and Story 1 scenarios pass.
2. **Given** at least one local-tier model, **When** the same cognition runs, **Then**
   Story 1 scenarios pass on that tier.
3. **Given** a tier/model that cannot tool-call reliably, **When** a cognition runs,
   **Then** the documented fallback engages and the cognition terminates with a recorded
   outcome.

---

### User Story 5 - The governor stays sane on multi-call cognitions (Priority: P3)

Today one cognition equals one metered model call; a tool loop is N calls per cognition.
The spend meter still records every billable call individually; the cognition governor's
estimates and calibration treat the whole loop as the unit a cognition budgets for, so
suppression arithmetic, drift telemetry, and the monthly budget ceiling all remain
truthful when cognitions become multi-call.

**Why this priority**: board AC#4. Wrong metering silently corrupts the governor's
suppression decisions and the budget ceiling — but it is calibration-layer work that can
land after the loop itself proves out.

**Independent Test**: run multi-call cognitions; verify (a) recorded spend equals the sum
of per-call costs, (b) the governor's per-cognition duration estimate converges on
whole-loop wall time, (c) the budget ceiling still refuses admission before any billable
call when exhausted.

**Acceptance Scenarios**:

1. **Given** a cognition whose loop made three billable calls, **When** spend is
   inspected, **Then** all three calls' costs are recorded.
2. **Given** a stream of multi-call cognitions, **When** the estimator converges,
   **Then** its per-cognition estimate reflects whole-loop wall time and the route
   verdict arithmetic remains a pure function of recorded observations.
3. **Given** an exhausted monthly budget, **When** a loop attempts its next billable
   call mid-cognition, **Then** admission is refused before any spend and the cognition
   terminates with a recorded failure outcome.

---

### Edge Cases

- Model replies with plain text and never calls any tool → the cognition ends with a
  recorded failure outcome (no silent drop); the fallback convention (Story 4) defines
  whether text is first interpreted under the fallback before being declared a non-answer.
- Model's acting call is rejected by the gate (stale, guard, scene, charge) → the
  rejection is recorded and fed back as the call's result; the model may attempt a
  different action within the remaining iteration cap (a rejected request does not spend
  the cognition's one action — only a landed one does).
- Model calls an unknown tool name, an off-roster tool, or malforms parameters → recorded
  rejected artifact; the error is fed back; the loop continues within the cap.
- Iteration cap reached with no acting call landed → recorded failure outcome; zero
  world mutation.
- Tier goes down / call times out / queue is full mid-loop → the cognition terminates
  with the same recorded failure outcome family as today's failed thoughts; any already-
  landed acting call stands (it is a fact); no retry re-runs a landed action.
- Budget ceiling trips between iterations (cloud) → next call refused pre-spend; loop
  terminates with recorded outcome.
- Two concurrent cognitions for different agents interleave tool calls → correlation
  identifiers keep their chains disjoint; no cross-agent inference needed.
- Replay encounters events emitted by a loop that crashed mid-cognition → replay applies
  exactly the events that were emitted; no reconciliation logic is needed because
  nothing durable ever awaited the loop's completion.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A cognition MUST be able to declare a set of tools drawn from the spec-014
  registry (a roster subset); the declared set, with parameter shapes derived from the
  registry's parameter metadata, is presented to the model as callable tools.
- **FR-002**: The system MUST run a bounded execute-and-feed-back loop: model responds
  with tool calls; the loop dispatches each call to its registered handler; results are
  fed back to the model; the loop continues until a terminal condition (acting call
  landed, model finishes, cap reached, or error).
- **FR-003**: The loop MUST enforce a hard iteration cap and terminate within it in all
  cases, including adversarial model behavior; cap exhaustion yields a recorded failure
  outcome and zero unlanded world mutation.
- **FR-004**: Cardinality MUST be one landed acting tool (World or Expressive effect
  class) per cognition. Read-effect tools are exempt: they may be called repeatedly
  within the cap as mid-loop lookups. After an acting call lands, any further acting
  call in the same cognition MUST be rejected and recorded. Rejected acting calls do not
  consume the cognition's action.
- **FR-005**: Tool handlers MUST honor the request/fact boundary: read-effect handlers
  return data and emit nothing; mutating handlers (World, Expressive) route through the
  existing landing doors so that every durable effect is an event admitted by the
  existing gates (landing ladder, whitelist dry-run). No handler mutates state directly.
- **FR-006**: Replay MUST NOT re-run any part of a tool loop: model transcripts and tool
  results are not replayed; only emitted events are. Replay of any log — including logs
  written before this feature — MUST reproduce byte-identical state and payloads.
- **FR-007**: Every tool call MUST be recorded as a first-class artifact carrying a
  correlation identifier (the cognition's job identifier plus a per-call ordinal),
  including calls that are rejected, malformed, off-roster, or never grounded. The
  record MUST include the verdict.
- **FR-008**: Grounding events caused by a tool call (intent-set, plan-set, and the
  expressive landings) MUST carry the causing job identifier as an additive payload
  field, absent-when-empty, so that `tool call → verdict → grounding chain` is queryable
  from the event log by identifier alone. Pre-existing events MUST remain byte-stable.
- **FR-009**: The loop MUST work on the cloud tier via the provider's native
  tool-calling interface and on at least one local-tier model. On the local tier the
  strategy is native-first: the local transport's native function-calling API is used
  where the configured model supports it reliably; otherwise the fallback convention
  (FR-010) engages, selected by per-model configuration.
- **FR-010**: For any tier or model that cannot tool-call reliably, exactly one
  documented fallback convention MUST exist — a provider-agnostic, schema-constrained
  structured-output convention that emulates a tool call round — and engaging it MUST
  still satisfy FR-003 through FR-008 (bounded, gated, recorded, correlatable).
- **FR-011**: Every billable model call in a loop MUST be metered individually (spend
  recorded per call; the budget ceiling checked before each billable call). The
  cognition governor's estimation and calibration MUST treat the whole loop as one
  cognition (points cover the loop; observed wall time is whole-loop), and its routing
  verdict MUST remain pure arithmetic over recorded observations.
- **FR-012**: The registry MUST gain a numeric parameter kind so integer parameters
  (the storage verbs' quantity) are representable in tool parameter metadata and thus in
  derived tool schemas (debt recorded in spec 014's tool catalog).
- **FR-013**: The `muse` expressive tool MUST be callable through the loop as one of the
  acting choices in the roster presented to villager cognitions, and the
  separately-scheduled musing channel MUST be removed in this task: musing occurs only
  when a cognition chooses the muse tool, so it carries the same opportunity cost as any
  other action.
- **FR-014**: Both the villager planner-class cognition and the metatron turn MUST adopt
  the loop in this task, each presenting its own registry roster (metatron's tools —
  including the charge-gated nudges — are already registered). Conversation scenes and
  nightly consolidation keep their current mechanics.
- **FR-015**: A cognition whose model output yields no valid tool call (plain text,
  after any fallback interpretation) MUST end with a recorded failure outcome — never a
  silent drop and never a hand-parsed side effect.

### Key Entities

- **Tool call (request)**: the model's utterance naming a tool and arguments; ephemeral
  in transcript, durable as a recorded artifact with job identifier + call ordinal and a
  verdict. Never itself a fact.
- **Tool result**: data fed back to the model for a call — read-tool data, a gate
  verdict, or an error description. Transcript-only; never replayed.
- **Acting tool**: registry tool with World or Expressive effect; landing one consumes
  the cognition's single action.
- **Read tool**: registry tool with Read effect (class exists since spec 014 with zero
  entries); returns data, emits nothing, exempt from cardinality.
- **Cognition (loop instance)**: one bounded loop run, identified by the existing job
  identifier; the unit of governor estimation; may contain N billable calls.
- **Correlation identifier**: job identifier (+ per-call ordinal) threading request
  artifact → verdict → grounding events.
- **Fallback convention**: the single documented alternative wire-shape for tiers/models
  without reliable native tool calling.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A cognition that performs at least one mid-loop read lookup and then lands
  an acting tool completes end-to-end on the cloud tier and on at least one local-tier
  model, terminating within the iteration cap 100% of the time in a soak of ≥100
  cognitions per tier.
- **SC-002**: Replay of a run containing tool-loop cognitions reproduces byte-identical
  state with zero tool-handler or model invocations during replay; the pre-existing
  replay/snapshot test suite passes unmodified against pre-feature fixture logs.
- **SC-003**: For 100% of tool calls in a run — landed, rejected, malformed, or never
  grounded — the call artifact and its verdict are retrievable from the event log by
  identifier alone, and for 100% of tool-caused grounding events the causing call is
  reachable the same way, with zero adjacency inference.
- **SC-004**: Recorded spend for a multi-call cognition equals the sum of its per-call
  costs exactly; with the budget exhausted, zero billable calls are admitted; governor
  drift telemetry over a soak shows estimates converging on whole-loop wall time (no
  permanent breach state from the loop change alone).
- **SC-005**: Agents keep acting after migration: over a fixed-length soak on each
  supported tier, the rate of landed intents per agent-hour is within normal variance of
  the pre-change baseline (no regression traceable to the loop mechanics).

## Assumptions

- The spec-014 registry is the sole source of tool identity, parameter shapes, gates,
  and rosters; this feature adds a dispatch/handler layer and a numeric parameter kind
  but does not restructure the registry.
- Handlers for existing world verbs and expressive tools wrap the existing inject doors;
  no new event types are required for grounding (the additive job-identifier field rides
  existing payloads). New artifact records for the calls themselves may introduce new
  telemetry event types, which — like today's cognition telemetry — are reducer no-ops.
- The loop runs at decision time (like today's planner calls), outside the deterministic
  tick loop; nothing in the sim tick awaits a loop.
- A rejected acting call is fed back and retryable within the cap; only a landed acting
  call consumes the cognition's single action (derived from "the gate decides" — the
  model may not know a target died; rejection-and-retry is the informative behavior).
- Failure outcomes reuse today's recorded outcome vocabulary (suppressed / rejected /
  failed families) rather than inventing a parallel one.
- TASK-16's journal tools (write/search/read journal) are out of scope: this feature
  ships the loop, the read-effect plumbing, and handler seams they will plug into.
- Conversation scenes and nightly consolidation keep their current mechanics (FR-014
  scope decision, session 2026-07-22).
- The iteration cap and any per-cognition call budget are operator-configurable with
  safe defaults; exact numbers are a plan-level decision.
