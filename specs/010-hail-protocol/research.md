# Research: Hail Protocol

All decisions below were derived from direct code reads (pinned at the current main,
post-4f045f2); no NEEDS CLARIFICATION markers remained in the Technical Context.

## D1 — Where the relaxation lives: the loop's landing ladder, not Guard.Eval

**Decision**: keep the guard vocabulary closed and `Guard.Eval` untouched. The
`inject_intent` handler in `internal/sim/loop.go` gains a hail rung: when a
`target_present` guard fails for a `talk_to` landing, evaluate the deterministic
`hailable` predicate; if it holds (or the target is the actor's own hailer — see D6),
the landing proceeds and is recorded with the existing **adapted** outcome instead of
`rejected-guard`.

**Rationale**: `guard.go` documents the guard set as closed and evaluation as a pure
predicate of state; teaching `target_present` about hails would leak protocol
semantics into the vocabulary and silently change every other caller. The ladder in
`loop.go:451-479` is already where outcome policy lives (staleness, generation,
adapt rung) — the hail is one more rung of the same policy.

**Alternatives considered**: (a) new `target_hailable` guard type authored by the
mind — rejected: the model must never author predicates, and the mind shouldn't need
changing at all; (b) widening `presentRadius` — rejected: it weakens the guard for
every goal and doesn't stop the target from continuing to walk.

## D2 — Hail on every hailable talk_to landing, not only out-of-radius ones

**Decision**: after a `talk_to` landing succeeds (guard held or hail rung fired),
emit `social.hailed` whenever the target is hailable. Applies to planner landings
(`loop.go`) and to plan-step `talk_to` firings (`plan.go` → `planStepEvents`).

**Rationale**: an in-radius target still walks during the hailer's approach (seek
targets the tile captured at landing; `resolveGoal` `policy.go:196-204`); the pause
is what makes arrival meet a person instead of an empty tile. Uniformity also makes
the feature's behavior easy to state and test. Bare `seek` goals do NOT hail —
`talk_to` is deliberate conversation intent; seek is just movement.

## D3 — Pause semantics: suppress movement only, in the executor's per-agent step

**Decision**: a paused agent (Agent.Hail non-nil, tick < Until) skips: the reflex
branch, plan-step evaluation, and the en-route movement branch. It does NOT skip:
needs decay, being a party to social events, or `executeAtTarget` when already
standing on its intent target (stationary work continues). Intent and Plan fields
are never written by the pause.

**Rationale**: FR-004 requires "no movement, nothing else disturbed." Stationary
work is physically compatible with being paused (the agent isn't going anywhere),
and skipping plan-step evaluation while paused means a held plan simply resumes —
`Until` windows (default 2 game-hours) dwarf the pause window so expiry-during-pause
is a non-issue in practice.

## D4 — Per-tick hail sweep for expiry AND meeting, not the %60 social beat

**Decision**: a dedicated `hailStep` sweep runs every tick before the per-agent
loop: for each agent with an active hail — if the hailer is adjacent (Manhattan ≤ 1)
emit `social.hail_met` + the same `agent.talked` / `social.relation_changed` /
memory event shape the ambient beat emits (bypassing `canTalk`); else if
`tick >= Until` emit `social.hail_expired`.

**Rationale**: the ambient beat fires at `tick%60 == 30` and requires distance
exactly 1 plus both cooldowns — three ways for a successful walk-over to found
nothing. A hailer completing `seek` stands ON the target's tile (distance 0), which
the ambient beat can never see. A per-tick sweep over 8 agents is effectively free,
makes met-vs-expired a race with a deterministic winner (met checked first), and
`agent.talked` still feeds the mind's conversation driver unchanged.

## D5 — Tunables

**Decision**: `hailRadius = 64` (Manhattan), `hailWindowTicks = 480` (8
game-minutes), constants beside the existing tuning table in `internal/sim`.

**Rationale**: observed guard failures land at distances 35–50, so 64 covers the
population with margin; walk speed is 12 tiles/game-minute (`moveEveryTicks = 5`),
so the far edge of hail range costs ~5.3 game-minutes of walking — 480 ticks gives
~50% margin for path detours. Denominated in game ticks, so wall speed changes are
irrelevant by construction (FR / edge case "speed changes mid-pause").

## D6 — Deadlock prevention: hailers are exempt, mutual hail resolves to presence

**Decision**: `hailable(s, hailer, target)` is false when the target itself has an
outstanding outbound hail (∃ k: `s.Agents[k].Hail.By == target`). Separately, when a
`talk_to` landing's target is the actor's own hailer (`actor.Hail.By == target`),
the landing is treated as present (adapted) WITHOUT emitting a new hail — the pair
is already converging.

**Rationale**: two frozen agents waiting on each other is the only deadlock shape;
making "actively hailing" an exemption from being paused breaks it structurally.
The scan is O(8).

## D7 — Un-interruptible states

**Decision**: `hailable` requires target alive, awake, not already hailed
(first-hail-wins), not an active hailer (D6), not meeting-pinned (Intent.Goal ==
"attend_meeting" or `meetingActive(s) && attendCandidate(s, target)`), and within
`hailRadius` of the hailer. Reducer also clears `Hail` on `agent.died` and
`agent.slept` (a target that dies or falls asleep mid-pause sheds the hail; the
hailer's seek proceeds/fails exactly as today).

**Rationale**: FR-009 exemptions plus two mid-pause state collisions found in the
executor read: the asleep branch runs before anything else (`executor.go:121-126`),
and the meeting pin overrides all intents (`executor.go:132-140`). Clearing on
sleep/death in the reducer keeps `Hail` from becoming dangling state that would
otherwise silently pause the agent after it wakes.

## D8 — Event shapes: `from`/`to` field names for free TUI/tail visibility

**Decision**: payloads use `from` (hailer) and `to` (target): `social.hailed {from,
to, until}`, `social.hail_met {from, to}`, `social.hail_expired {from, to}`.
No TUI changes. The three types are NOT added to `injectSocialWhitelist`.

**Rationale**: the chronicle grammar (`internal/tui/grammar.go`) renders unknown
event types generically and already resolves `from`/`to` integer fields to agent
names — visibility (FR-008) lands with zero view-layer surface. Keeping the types
out of the whitelist preserves the isolation invariant: hails are world-emitted
facts, never model-injectable.

## D9 — Snapshot compatibility

**Decision**: `Agent.Hail *AgentHail` (`{by int, until int64}`) with
`json:"hail,omitempty"`.

**Rationale**: pointer + omitempty keeps canonical state bytes identical for
pre-feature snapshots (the same pattern as `State.Gru` and
`State.MeetingConvention`), satisfying FR-010 and the determinism-hash tests.

## D10 — Measurement protocol for SC-001/SC-002

**Decision**: before/after on the same world shape as the recorded baseline
(local tier, 8x+ speed, myworld-01 config): count `agent.intent_rejected` with
reason containing "is gone" and `social.conversation` events over a fixed
wall-clock window; record both counts on TASK-47.

**Rationale**: TASK-47 already pins the baseline (1 conversation in ~75 min;
4× rejected vs 1× conversation); reusing the same shape makes the ≥70% reduction
claim (SC-001) an apples-to-apples query over the event log.
