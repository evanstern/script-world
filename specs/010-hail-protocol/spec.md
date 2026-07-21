# Feature Specification: Hail Protocol

**Feature Branch**: `010-hail-protocol`

**Created**: 2026-07-21

**Status**: Draft

**Input**: User description: "Hail protocol: when an agent forms a talk_to intent toward another agent, emit a cheap deterministic sim-level 'hail' ('let's chat') that pauses the target in place for a short tunable window so the hailer can close distance and the conversation can start. Today talk_to goals carry a target_present guard (presentRadius=16 Manhattan) checked at landing; planner LLM calls take wall-clock seconds, and at 8x+ game speed the target routinely walks beyond the radius before the intent lands, so the thought is rejected ('X is gone (distance N)') and the LLM spend is wasted — agents rarely open conversations at speed. Source: Backlog TASK-47."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A talk_to decision survives target movement at speed (Priority: P1)

A villager's planner decides to talk to another villager. By the time that decision
lands in the world (the planner call took wall-clock seconds spanning many game ticks),
the target has walked well beyond the present radius. Instead of the thought being
rejected and the LLM spend wasted, the world delivers a hail — "let's chat" — to the
target: the target stops in place, the hailer's intent lands as an adapted walk toward
the target's current position, and the two meet and talk.

**Why this priority**: this is the defect being fixed — at 8x+ speed on the local tier,
talk_to decisions almost always die at the landing guard (baseline: 1 conversation in
~75 wall-minutes while talk_to attempts failed at distances 35–50). Without this story
there is no feature.

**Independent Test**: in a world where agent B is beyond the present radius from agent A
but within hail range, inject a talk_to(A→B) intent with landing metadata. Verify the
intent lands (adapted, not rejected), a hail event is recorded, B stops moving, A walks
to B, and a talk between A and B is founded on adjacency.

**Acceptance Scenarios**:

1. **Given** a talk_to intent whose target has moved beyond the present radius but is
   within hail range and interruptible, **When** the intent lands, **Then** the landing
   is accepted (recorded as adapted), a hail event is recorded, and the target enters a
   paused state.
2. **Given** a hailed, paused target, **When** the hailer walks to it and becomes
   adjacent within the pause window, **Then** a talk between the pair is founded
   deterministically and the hail is cleared.
3. **Given** a talk_to intent whose target is within the present radius, **When** the
   intent lands, **Then** it lands as today AND the target is also hailed (paused), so
   the target does not wander off while the hailer closes the remaining distance.
4. **Given** a talk_to intent whose target is beyond hail range, or dead, **Then** the
   landing is rejected exactly as today, with the existing reason.

---

### User Story 2 - A stood-up target resumes its life safely (Priority: P2)

A villager is hailed and stops to hear the other out, but the hailer never arrives
(died, fell asleep, was unreachable, or the world simply moved on). After the pause
window expires the villager resumes exactly what it was doing — same intent, same
pending plan — having lost nothing but the wait.

**Why this priority**: without safe expiry the hail is a denial-of-service on the
target's autonomy; the pause must be bounded and non-destructive or the cure is worse
than the disease.

**Independent Test**: hail a target whose hailer never moves toward it; advance the
clock past the pause window; verify a hail-expired event is recorded, the target's
prior intent and plan are untouched, and the target moves again on subsequent ticks.

**Acceptance Scenarios**:

1. **Given** a paused target with an in-progress intent (e.g. walking to forage),
   **When** the pause window expires without the hailer arriving, **Then** a
   hail-expired event is recorded and the target resumes the same intent from where it
   stood.
2. **Given** a paused target holding a pending multi-step plan, **When** the pause
   expires, **Then** the plan is still intact (no steps dropped, no plan cleared).
3. **Given** a paused target, **When** the pause window has not yet expired, **Then**
   the target does not move, does not take new movement intents, but its needs continue
   to evolve normally (the world does not freeze around it).

---

### User Story 3 - Un-interruptible villagers are left alone (Priority: P2)

A villager who is asleep, dead, or attending the village meeting cannot be flagged
down by a hail. The hail protocol respects states where stopping-to-chat makes no
sense, and the landing path treats those targets exactly as it does today.

**Why this priority**: pausing a sleeping agent is meaningless, and pulling an agent
out of the meeting pin would corrupt the governance loop. Exemptions are a correctness
constraint, not a nice-to-have.

**Independent Test**: attempt hails against an asleep target, a dead target, and a
meeting attendee; verify no hail event is recorded for any of them and the talk_to
landing falls back to today's behavior (lands if within present radius, rejects
otherwise).

**Acceptance Scenarios**:

1. **Given** a target that is asleep, **When** a talk_to intent lands, **Then** no hail
   is recorded and the landing follows today's present-radius rule.
2. **Given** a target pinned to an active meeting, **When** a talk_to intent lands,
   **Then** no hail is recorded and the target keeps attending.
3. **Given** a target already paused by an earlier hail from someone else, **When** a
   second talk_to lands against it, **Then** the target is treated as present (it is
   standing still), the second landing succeeds, but the pause window is not extended.

---

### User Story 4 - Hails are visible to the observer (Priority: P3)

A person watching the world through the event tail or the TUI sees hails happen: who
hailed whom, and whether the hail was met or expired. The social texture of "flagging
someone down" becomes part of the observable story.

**Why this priority**: observability is how the feature's effect is measured (before/
after rejection counts) and how future tuning is grounded; it does not change world
behavior.

**Independent Test**: run a world where a hail occurs and expires; verify the hail and
hail-expired events appear in the event log with agent attribution and render in the
live tail.

**Acceptance Scenarios**:

1. **Given** a hail is emitted, **Then** an event naming hailer and target is appended
   to the event log and visible in the live tail.
2. **Given** a hail expires unmet, **Then** a distinct expiry event is appended and
   visible in the live tail.

---

### Edge Cases

- **Mutual hail / deadlock**: A hails B (B pauses); B's own in-flight thought lands
  wanting to talk to A. A is en route to a target it hailed and is exempt from being
  paused — an agent actively answering its own hail can never be frozen by an incoming
  one. B's talk_to lands against a moving-toward-it A (A is treated as present); the
  pair still meets. Two agents must never end up mutually frozen.
- **Hailer dies or falls asleep en route**: no special handling — the pause simply
  expires on schedule and the target resumes.
- **Target's own thought lands mid-pause**: the landing proceeds normally (intent or
  plan is set), but movement stays suppressed until the pause ends; the new intent
  executes afterward.
- **Ambient talk cooldown on arrival**: the pair may have talked recently (ambient talk
  cooldown not yet elapsed). A hail-founded meeting is deliberate planner intent, not
  ambient chatter: adjacency between hailer and hailed target founds the talk regardless
  of the ambient cooldown (the cooldown timestamps update as usual afterward).
- **Arrival geometry**: "arrival" is adjacency (Manhattan distance ≤ 1), not standing on
  the target's tile; the meeting must found even if pathing would otherwise step onto or
  stop short of the target's exact tile.
- **Recovery/replay**: hail state (who is paused, by whom, until when) must reconstruct
  identically from the event log — a replayed world pauses and resumes the same agents
  at the same ticks.
- **Speed changes mid-pause**: the pause window is denominated in game ticks, so wall
  speed changes (or pause/resume of the whole world) neither stretch nor shrink it in
  game terms.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When a talk_to intent lands and its target is hailable, the system MUST
  record a hail (hailer, target, expiry) and put the target into a paused state, on
  every such landing — including landings where the target is still within the present
  radius.
- **FR-002**: A talk_to landing whose target has moved beyond the present radius but is
  hailable and within hail range MUST be accepted (recorded with the adapted outcome)
  instead of rejected; the landing resolves toward the target's current position.
- **FR-003**: "Hailable" MUST be a deterministic predicate of current world state:
  alive, awake, not pinned to an active meeting, not already paused by a hail, not
  itself en route answering its own hail, and within hail range of the hailer. Hail
  range MUST be a tunable constant that covers the observed failure distances (35–50
  tiles) by default.
- **FR-004**: A paused target MUST NOT move or begin new movement while the pause is
  active; its needs, incoming social interactions, and any stationary work at its
  current tile MUST continue unaffected. The pause MUST NOT clear or modify the
  target's current intent or pending plan.
- **FR-005**: The pause MUST expire after a tunable window denominated in game ticks
  (default: a few game-minutes, sized so a hailer at maximum hail range can walk the
  distance with margin). On expiry an expiry event MUST be recorded and the target
  MUST resume its prior behavior with no other state change.
- **FR-006**: When the hailer becomes adjacent (Manhattan ≤ 1) to its hailed target
  before expiry, the pair's talk MUST be founded deterministically at the next social
  beat, bypassing the ambient talk cooldown for that founding, and the hail MUST clear.
- **FR-007**: The hail path MUST be entirely deterministic sim logic: zero LLM calls,
  no wall-clock reads, no randomness beyond the world's seeded streams. All hail state
  transitions MUST be event-sourced through the reducer so replay and recovery
  reconstruct identical state.
- **FR-008**: Hail lifecycle events (hailed, expired) MUST carry agent attribution and
  be visible through the standard event log surfaces (tail, TUI event stream).
- **FR-009**: Targets in un-interruptible states (asleep, dead, meeting-pinned) MUST
  never be hailed; talk_to landings against them MUST behave exactly as before this
  feature.
- **FR-010**: Hail/pause state MUST round-trip through world snapshots: a world
  restarted mid-pause resumes the pause with the same expiry.

### Key Entities

- **Hail**: a courtesy signal from one agent (the hailer) to another (the target),
  created when a talk_to intent lands; carries hailer, target, and expiry tick; ends by
  being met (adjacency) or by expiry.
- **Pause state (per agent)**: the target-side effect of a hail — who paused this
  agent and until what tick; suspends movement only; orthogonal to the agent's intent
  and plan, which it preserves.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On the baseline world shape (local tier, 8x+ speed), talk_to landings
  rejected with reason "is gone" drop by at least 70% over a comparable observation
  window (before/after measurement recorded on the board task).
- **SC-002**: Conversations founded per game-day on the baseline world shape increase
  measurably over the recorded baseline (1 conversation in ~75 wall-minutes).
- **SC-003**: A hailed target whose hailer never arrives resumes its prior intent and
  plan within one tick of pause expiry, with intent and plan byte-identical to
  pre-pause state.
- **SC-004**: Replaying an event log containing hails, expiries, and hail-founded talks
  reproduces the identical world state hash as the live run.
- **SC-005**: Zero LLM requests are issued by the hail path (verifiable in tests by
  absence of orchestrator traffic attributable to hails).

## Assumptions

- A hail is emitted on **every** hailable talk_to landing, not only out-of-radius ones:
  even an in-radius target can wander during the walk-over, and the courtesy pause is
  cheap. (Derived from the task's intent; makes behavior uniform.)
- A hail-founded meeting bypasses the ambient talk cooldown: the planner deliberately
  chose this conversation, and the cooldown exists to bound ambient chatter, not
  deliberate intent. Without the bypass, a successful hail could still produce zero
  conversation — defeating the feature's purpose.
- First hail wins: a second hail against an already-paused target does not extend the
  pause or reassign the hailer. Prevents pause-chaining from freezing an agent
  indefinitely.
- An agent en route to a target it hailed is exempt from being paused itself
  (deadlock prevention takes priority over courtesy symmetry).
- Sleeping targets are not hailed but are also naturally stationary; the landing rule
  for them is unchanged (present-radius). Accepting out-of-radius landings against
  sleepers is out of scope.
- Default tunables (hail range, pause window) are constants in the sim's tuning table
  like existing cadences/radii; runtime configurability is out of scope.
- Rejections whose cause is not target distance (dead target, actor asleep, staleness
  budget, superseded generation) are out of scope and unchanged.
