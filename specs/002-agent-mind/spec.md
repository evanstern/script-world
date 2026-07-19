# Feature Specification: Agent Mind v1

**Feature Branch**: `task-7-agent-mind`

**Created**: 2026-07-19

**Status**: Draft

**Input**: User description: "The thinking layer: persona.md (immutable, never in any write path) + soul.md (sim-written, player-readable) per agent per run; top-K working memory (reverse-chron, cheap rerank, serendipity mix from the tail); planner calls on 30-game-min cadence + scene-change triggers producing structured intents for the executor. Grounding: grounded-assumptions.md (Agent mind). Spec candidate #2, linked to Backlog TASK-7."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Villagers think for themselves (Priority: P1)

Eight named villagers each carry an authored, fixed nature (persona) and, on a
30-game-minute cadence plus scene changes (waking, going idle, night falling, meeting
someone), consult a local language model to decide what to do next. The decision
arrives as a structured goal the deterministic executor carries out. When the model is
slow, down, or over budget, the survival reflex quietly takes over — villagers never
freeze.

**Why this priority**: "The prompt is the behavior" is the game's founding idea; this
story is the first time model output steers a body. Everything later (souls read in
play, Metatron whispers, consolidation) assumes this loop exists.

**Independent Test**: Run a world against a mock local model that returns fixed goals;
observe planner-sourced intents appearing on cadence and on triggers, and the executor
acting on them. Kill the model; observe reflex behavior resume after a short grace.

**Acceptance Scenarios**:

1. **Given** a running world with a reachable local model, **When** a game half-hour
   passes for an agent, **Then** a planner call happens for that agent and its chosen
   goal appears in the event log as a planner-sourced intent the executor executes.
2. **Given** an agent that wakes, completes a task, or is present when night falls,
   **When** the trigger fires, **Then** a planner call happens without waiting for the
   half-hour cadence.
3. **Given** an unreachable model, **When** an agent sits idle past the reflex grace
   period, **Then** the survival reflex issues an intent — the village degrades to
   TASK-5 behavior, never to paralysis.
4. **Given** any replay of the event log, **Then** no model is ever called — planner
   decisions were recorded as inputs and replay reproduces the run exactly.

---

### User Story 2 - Natures are fixed; souls grow (Priority: P2)

Each agent's save directory holds two documents: `persona.md`, authored at world
creation and never modified by anything afterward, and `soul.md`, a human-readable
record the simulation keeps appending to — episodic memories with day/time and a
salience weight — that the player can open and read at any time.

**Why this priority**: The persona/soul split is THE core data model from the
grounding session ("souls change, natures don't"), and the persona firewall is a named
mechanization requirement. It must exist before consolidation (TASK-9) has anything to
rewrite or Metatron (TASK-12) anything to read.

**Independent Test**: Create a world; hash every persona.md; run days of simulation;
hashes unchanged and files read-only, while each soul.md has accreted dated,
salience-weighted memories.

**Acceptance Scenarios**:

1. **Given** a new world, **Then** eight `agents/<name>/persona.md` files exist with
   authored content and are read-only on disk; no code path writes them after creation.
2. **Given** memorable happenings (a conversation, a build, a near-death, a death
   witnessed), **When** they occur, **Then** matching memories appear in that agent's
   `soul.md` with game-day timestamps and salience, and survive daemon restarts.
3. **Given** a copied save directory, **Then** personas and souls travel with it (flat
   files bound to the run).

---

### User Story 3 - Minds run on a window, not the whole soul (Priority: P3)

A planner call never sees an agent's entire history. Working memory is a bounded
top-K selection: recent and salient memories dominate, with a couple of old
"serendipity" memories mixed in from the tail so long-buried moments can resurface.

**Why this priority**: The grounding session explicitly rejected whole-soul-in-context
(cost and focus); the window is what keeps ~3,800 daily local calls affordable and
souls unbounded. It's a P3 because it refines Stories 1–2 rather than standing alone.

**Acceptance Scenarios**:

1. **Given** an agent with far more memories than the window size, **When** a planner
   prompt is built, **Then** it contains at most K memory lines — top-scoring by
   salience-and-recency, plus a fixed number of tail picks.
2. **Given** the same agent state and tick, **Then** window selection is identical on
   every run (deterministic rerank; serendipity picks are seeded, not random).

---

### Edge Cases

- **Model returns garbage**: unparseable planner output is discarded (recorded as a
  failed thought, no intent); the reflex grace period covers the gap.
- **Model chooses the impossible**: a goal that cannot be resolved (no such resource
  reachable, unknown goal name) is rejected at injection; the agent stays idle until
  the next trigger or reflex.
- **Planner overrides in flight**: a planner decision arriving while the agent is
  mid-intent replaces the current intent (minds outrank reflexes).
- **Budget/queue pressure**: orchestrator refusals (queue full, tier down) are treated
  exactly like an unreachable model — skip, retry next trigger, reflex covers.
- **Death**: dead agents get no planner calls and accrete no memories; their soul.md
  records the death and freezes.
- **Sleep**: sleeping agents don't plan; waking is itself a trigger.
- **Pause**: a paused world fires no cadence (game time isn't advancing); injections
  that arrive during pause apply at the boundary like any command.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST seed eight named agents at world creation, each with an
  authored `persona.md` and an initially-empty `soul.md` under
  `agents/<name>/` in the save directory.
- **FR-002**: `persona.md` MUST be structurally immutable: written exactly once at
  world creation, marked read-only on disk, with no post-genesis write path anywhere
  in the system.
- **FR-003**: The simulation MUST record episodic memories as durable events (agent,
  text, salience, tick) for memorable happenings — at minimum: conversations, builds,
  hunts, near-death, deaths witnessed, and finding food while starving.
- **FR-004**: `soul.md` MUST be regenerated from recorded memories as a
  player-readable document (game-day timestamps, salience) while the daemon runs, and
  MUST be reconstructible from the event log alone.
- **FR-005**: Working memory MUST be a bounded top-K selection over an agent's
  memories — scored by salience and recency, reverse-chronological presentation, with
  a fixed quota of deterministic tail picks — and planner prompts MUST use only this
  window, never the full memory list.
- **FR-006**: The system MUST request a planner decision for each living, awake agent
  on a staggered 30-game-minute cadence, and additionally on scene changes: waking,
  becoming idle after completing an intent, night falling, and first encounter with
  another agent (with a cooldown).
- **FR-007**: Planner calls MUST route through the LLM orchestrator's local tier as
  kind `planner`, with the persona as the stable (cacheable) prompt prefix and the
  memory window + current situation as the variable suffix.
- **FR-008**: Planner output MUST be a structured goal drawn from the executor's
  action vocabulary (forage, chop, hunt, build fire/shelter, eat, sleep, wander, seek
  warmth, seek out a named agent); the system MUST resolve it to a concrete target
  deterministically at the tick boundary and record it as a planner-sourced intent
  event plus a thought event carrying the model's stated reason.
- **FR-009**: Model output MUST enter the simulation only as recorded events; replay
  MUST reproduce a run byte-for-byte without any model call.
- **FR-010**: When no planner decision arrives (model down, over budget, queue full,
  garbage output), an agent idle beyond a short grace period MUST fall back to the
  TASK-5 reflex policy; the reflex MUST remain the permanent degraded mode.
- **FR-011**: Souls and personas MUST live inside the save directory and travel with
  it (copy of a stopped world carries the villagers whole).

### Key Entities

- **Persona**: authored, immutable nature — temperament, drives, quirks. File on
  disk; never in any write path after genesis.
- **Memory**: one episodic record — text, salience (1–10), tick. Event-sourced into
  state; rendered into soul.md.
- **Working-memory window**: derived, bounded selection of memories for one planner
  call; pure function of (agent state, tick, K).
- **Thought**: a planner (or reflex) decision record — the goal chosen and the stated
  reason; feeds the chronicle later.
- **Mind driver**: daemon-side component that watches the event stream, schedules
  cadence/triggers, calls the orchestrator, and injects decisions as commands.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In a running world with a (mock or real) local model, every living agent
  produces at least one planner-sourced intent per 30 game minutes of awake time, and
  trigger events (wake, idle, nightfall) each produce a planner call within one
  game-minute.
- **SC-002**: After multiple simulated days, every persona.md hash is byte-identical
  to genesis and every file remains read-only; every soul.md contains dated,
  salience-weighted memories reflecting logged happenings.
- **SC-003**: No planner prompt ever contains more than K memory lines (K
  configurable, default 10), regardless of soul size; window selection for identical
  state is identical across runs.
- **SC-004**: With the model killed mid-run, no agent stays idle longer than the
  reflex grace (2 game-minutes) while awake; village survival behavior continues
  (TASK-5 guarantees hold).
- **SC-005**: A replayed event log reproduces the exact run (state hash equality) with
  zero model calls.
- **SC-006**: The whole loop runs on the local tier within the existing orchestrator
  backpressure — planner traffic alone never opens the cloud meter.

## Assumptions

- **Decided stack context**: TASK-5 executor supplies the action vocabulary and
  reflex; TASK-6 orchestrator supplies routing, degraded mode, and backpressure. This
  feature adds no new external dependencies.
- **Eight agents** replace the four placeholder bodies; personas are authored
  in-repo as v1 defaults (player-authored personas are post-v1).
- **Cheap rerank is deterministic scoring** (salience × recency decay), not an LLM
  call; per-agent learned rerankers stay parked per the grounding session.
- **Nightly consolidation (TASK-9) is out of scope**: souls only accrete here;
  compression/rewriting arrives with the consolidation pass, as is the persona-drift
  validator (the firewall here is structural + filesystem, which TASK-9 extends with
  content validation).
- **Conversations stay the TASK-5 primitive** (adjacency talk events); multi-turn
  LLM conversations are TASK-8 territory.
- **Memory growth is bounded in practice** by consolidation later; v1 accepts
  unbounded accretion within a 30-day run's scale (hundreds of memories per agent).
