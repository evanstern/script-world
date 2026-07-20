# Feature Specification: Nightly Consolidation + Persona Firewall

**Feature Branch**: `task-9-consolidation`

**Created**: 2026-07-19

**Status**: Draft

**Input**: User description: "Nightly consolidation + persona firewall: at each agent's sleep, one cloud-tier LLM call per agent per game night compresses that day's episodic memory buffer into durable soul updates — memories promoted or faded, beliefs revised with confidence and provenance, and a short self-narrative rewritten in the agent's own voice. The firewall is mechanized two ways: structurally, the consolidator has no write path to persona.md (natures don't change); and an automated validator inspects every consolidation output and rejects temperament drift before anything lands. Consolidation results enter the world only as recorded events applied by the reducer, so replay never re-calls a model. Souls must visibly grow across a multi-day run. Grounding: docs/design/grounded-assumptions.md (Agent mind). Backlog TASK-9, depends on TASK-7 (agent mind) and TASK-6 (orchestrator cloud tier)."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Sleep consolidates the day (Priority: P1)

When a villager settles in to sleep for the night, their mind quietly digests the day: the flood of small moments gets compressed — a few significant memories are strengthened so they endure, trivial ones fade toward forgetting, and the day itself is captured as a single remembered gist. The next morning the villager wakes with a curated past instead of an ever-growing pile, and the player can open that villager's soul and see the digestion happened.

**Why this priority**: This is the core loop of the feature and the first real consumer of the cloud tier. Without compression, memory windows silt up with noise and every downstream feature (beliefs, narrative, drama) reads from mud. It alone delivers value: souls stay legible over long runs.

**Independent Test**: Run a world through one full game day with agents accumulating memories; when an agent sleeps, verify exactly one consolidation lands for that agent that night — some memories strengthened, some faded, a day-gist added — all as recorded events, visible in the soul file.

**Acceptance Scenarios**:

1. **Given** an agent with a day's worth of accumulated memories, **When** that agent falls asleep for the night, **Then** exactly one consolidation is performed for that agent that game night, and its effects (strengthened memories, faded memories, a day-gist memory) land as recorded events.
2. **Given** an agent whose consolidation already ran this game night, **When** the agent wakes and sleeps again the same night, **Then** no second consolidation is performed.
3. **Given** the higher-quality remote model tier is unreachable or its budget is exhausted, **When** an agent sleeps, **Then** the night passes without consolidation, the world keeps running, and the un-consolidated memories remain intact for the next night's attempt.
4. **Given** a world whose history contains consolidations, **When** the world is replayed from its event log, **Then** the resulting state is identical and no model is consulted during replay.

---

### User Story 2 - Souls that grow: beliefs and self-narrative (Priority: P2)

Beyond compressing memories, the nightly digestion revises what the villager *believes* — durable convictions with a confidence level and a note of where they came from ("Cedar breaks his word — I saw it myself"; "the woods past the river are dangerous — Hazel told me") — and rewrites a short self-narrative in the villager's own voice: who they are becoming, what this chapter of their life is about. Over a multi-day run, a player reading a soul sees a life story thickening, not a log accumulating.

**Why this priority**: This is what makes souls feel alive and is the payoff the player reads. It builds directly on US1's nightly call (same output, more fields) but is separable: compression works without beliefs.

**Independent Test**: Run three consecutive game nights of consolidation for one agent; verify the soul file afterward contains beliefs with confidence and provenance and a self-narrative in the agent's voice that references events from at least two different days.

**Acceptance Scenarios**:

1. **Given** a day in which an agent witnessed a promise being broken, **When** the night's consolidation runs, **Then** the soul may gain or revise a belief about the promise-breaker carrying a confidence level and provenance (witnessed, told-by-whom, or inferred).
2. **Given** three consecutive nights of consolidation, **When** the player reads the agent's soul, **Then** it contains a self-narrative in the agent's voice that has changed across the nights and references events from at least two distinct days.
3. **Given** a belief the agent already holds, **When** the day's events contradict it, **Then** consolidation may lower its confidence or revise it — with the revision recorded like any other change.

---

### User Story 3 - The firewall holds: natures don't change (Priority: P2)

A villager's nature — temperament, core disposition, the authored self — is permanent. The nightly digestion, however creative the model gets, must be structurally unable to touch the persona, and an automated validator inspects every consolidation before it lands: output that tries to bend the villager's temperament (the gentle one becoming cruel, the coward becoming fearless overnight) is rejected wholesale, that night's changes are discarded, and the rejection itself is recorded so the player can see the firewall working.

**Why this priority**: "Souls change, natures don't" is a core design commitment of the project. It must ship in the same release as consolidation — an unguarded consolidator even for a few days could corrupt souls in ways that are hard to unwind — but it is testable independently by feeding the validator a deliberately drifting output.

**Independent Test**: Feed the validation stage a fixture consolidation whose narrative or beliefs contradict the agent's authored temperament; verify it is rejected, zero soul changes land, and the rejection is recorded. Verify by construction that no consolidation path can modify a persona.

**Acceptance Scenarios**:

1. **Given** a consolidation output that recasts the agent against their authored temperament, **When** validation inspects it, **Then** the entire output is rejected, no changes land that night, and the rejection is recorded with a reason.
2. **Given** any consolidation, accepted or rejected, **When** it completes, **Then** the persona file is byte-identical to before — there is no code path by which consolidation writes a persona.
3. **Given** a rejected consolidation, **When** the next night arrives, **Then** the un-consolidated memories are still present and consolidation is attempted again fresh.

---

### Edge Cases

- **Agent dies during the day**: the dead receive no consolidation; their soul remains as last written.
- **Daemon restarts mid-night**: the once-per-night guarantee holds across restarts — whether a given agent's consolidation already happened is recoverable from the recorded history, not from process memory.
- **Empty day**: an agent with no new memories since the last consolidation is skipped — no call is spent on nothing.
- **Multi-day backlog**: after nights missed to outages or rejections, the next successful consolidation digests the entire accumulated buffer, not just the last day.
- **Speed changes**: "night" is game time; consolidation cadence follows the game clock at any speed setting.
- **Oversized day**: a day's buffer that would exceed the call's practical input size is truncated oldest-first for the call, but the un-sent memories still fade only by the normal aging rules — never silently dropped.
- **Model returns malformed output**: treated exactly like a validator rejection — nothing lands, recorded, retry next night.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST perform at most one consolidation per living agent per game night, triggered by that agent's sleep, and MUST NOT re-consolidate an agent whose consolidation for the current night already succeeded — including across daemon restarts.
- **FR-002**: Consolidation MUST run on the remote high-quality model tier and MUST respect that tier's budget ceiling and health state: an unavailable tier degrades to "skip tonight, retain the buffer," never to blocking the simulation.
- **FR-003**: A consolidation's input MUST be the agent's episodic buffer — memories accumulated since that agent's last successful consolidation — together with the agent's authored persona, current soul context, and relationship summary.
- **FR-004**: A successful consolidation MUST be able to: strengthen chosen memories (promote), weaken chosen memories (fade), add a single day-gist memory, add or revise beliefs (statement, confidence, provenance), and replace the agent's self-narrative — and MUST NOT be able to do anything else.
- **FR-005**: All consolidation effects MUST enter the world exclusively as recorded events applied by the same state-transition rules used everywhere else; replaying a world's history MUST reproduce identical state without consulting any model.
- **FR-006**: All effects of one consolidation MUST land atomically: either the entire accepted output is recorded, or nothing is.
- **FR-007**: There MUST be no code path by which consolidation modifies a persona (structural firewall).
- **FR-008**: An automated validator MUST inspect every consolidation output before it lands and MUST reject the entire output when it drifts from the agent's authored temperament; rejections MUST be recorded with a reason and MUST leave the episodic buffer intact for the next attempt.
- **FR-009**: The rendered soul MUST present the consolidated life: enduring memories, beliefs with confidence and provenance, and the current self-narrative, readable by the player at any time.
- **FR-010**: Consolidation activity (ran, skipped, rejected, cost) MUST be observable by the operator without reading raw storage.

### Key Entities

- **Episodic buffer**: the set of an agent's memories accumulated since their last successful consolidation; input to the night's digestion.
- **Belief**: a durable conviction held by an agent — statement, confidence level, provenance (witnessed / told-by / inferred, with source), subject agent where applicable; revisable by later consolidations.
- **Self-narrative**: a short prose passage in the agent's voice describing who they are becoming; wholly replaced by each accepted consolidation.
- **Consolidation record**: the per-agent, per-night outcome — accepted (with its effects) or rejected/skipped (with reason); the once-per-night ledger.
- **Persona (temperament)**: the authored, immutable nature; the validator's reference standard and the structural firewall's protected object.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Over a three-game-day run with the remote tier healthy, every living agent accumulates exactly three accepted consolidations — never two in one night, never zero in a healthy night with a non-empty buffer.
- **SC-002**: A deliberately temperament-drifting consolidation output is rejected 100% of the time in test, with zero soul changes landing and a recorded rejection reason.
- **SC-003**: After three consecutive game nights, each agent's soul contains beliefs with confidence and provenance plus a self-narrative referencing events from at least two distinct days — and the soul file is shorter than the raw un-consolidated memory log would have been.
- **SC-004**: Replaying a world whose history includes consolidations reproduces byte-identical state with zero model calls.
- **SC-005**: With the remote tier down for a full game night, the world runs uninterrupted; the following healthy night, every affected agent consolidates their multi-day backlog in a single call each.

## Assumptions

- One game night ≈ 6 real hours at default 4x speed; 8 villagers means ≈ 32 remote calls per real day — comfortably inside the monthly budget ceiling from the grounding session (≈ $34/month estimate at v1 scale on default pricing).
- The validator is deterministic and mechanical (no second model call to judge the first): it checks structural invariants and temperament keywords/tone bounds derivable from the authored persona. A model-judged validator is a future refinement.
- The existing memory-aging rules continue to apply between nights; consolidation adjusts salience but does not replace aging.
- Sleep already exists as an observable moment in the simulation (agents sleep nightly per the survival rules from the executor layer).
- The player reads souls through the existing soul files and TUI; no new reading surface is required.
- Dead agents' souls freeze as last written; no posthumous consolidation.
