# Research: Norms and Votes

Decisions resolving every open point in the Technical Context. Ground truth: the
existing codebase (pinned notes in `docs/wiki/`), grounded-assumptions.md, and the
spec's recorded assumptions.

## R1 — Where governance lives

**Decision**: the deterministic core lives in `internal/sim/governance.go` as
executor-emitted events + reducer arms — the gru/consolidation-marker precedent, not a
daemon component. The only model-facing piece (proposal phrasing) is a small driver in
`internal/mind` (`meeting.go`) beside the convo driver.

**Rationale**: SC-007 requires a full governance day with the model off. Anything the
meeting *needs* must therefore be a pure function of (state, map, tick) inside the
loop. The mind already owns "villager-voiced model calls injected atomically"
(convo driver); phrasing a proposal is exactly that shape and reuses the mind's
replica, absorb loop, single-flight guards, and `SocialInjector` seam.

**Alternatives considered**: a daemon-wired meeting component like Metatron
(rejected: meetings must run with no `llm.json` present; components in the LLM-gated
block don't); putting vote logic in the mind with model-decided votes (rejected:
violates the spec's determinism assumption and makes replay model-dependent).

## R2 — Meeting schedule and the noon anchor

**Decision**: three executor beats keyed off `clock.SecondOfDay(nextTick)`:

- `meetingConveneSecond = 11*3600 + 1800` (11:30) — emit `meeting.convened`; from
  here until close, awake non-exiled villagers are pinned toward the meeting place.
- `meetingOpenSecond = 12*3600` (43200, noon) — emit `meeting.opened` with the
  attendance snapshot (living, awake, within `meetingRadius` of the place).
- Close at `open + meetingTimeboxTicks (3600)`, extended to at most
  `+ meetingGraceTicks (900)` only if a speaking turn's proposal beat is still
  pending — emit `meeting.closed`. Early close when every attendee has had their turn.

Once-per-day guard: `State.Meeting.LastMeetingDay` vs `DayIndex(tick) = tick/86400 + 1`
(the `NightIndex` pattern), advanced by the `meeting.opened` reducer.

**Rationale**: identical to the proven day/night boundary emission
(`executor.go:25-42`) and the consolidation once-per-night marker
(`consolidate.go:38`). 30 game-minutes of convening at 1 tile/5 ticks = 360 tiles of
travel — any villager reaches any point on the map in time. Noon is structurally
gru-free.

**Alternatives considered**: wall-clock or driver-side scheduling (rejected: not
replayable); convening at dawn for an all-day agenda (rejected: spec pins noon).

## R3 — The meeting place

**Decision**: event-sourced. On the first convene beat, the executor derives the spot
deterministically — the village's first fire structure's tile if one exists, else the
first shelter's, else the map-center-nearest passable tile — and emits
`meeting.place_designated{X,Y}` exactly once; `State.MeetingPlace *Point` persists it
forever after (structures burning down or being replaced never move it).

**Rationale**: the map has no landmarks by design (cold start); structures are the
only named places. "The village gathers where the first fire was lit" is both
deterministic and diegetic. Persisting the choice keeps every later meeting at the
same spot regardless of structure churn.

**Alternatives considered**: recompute each day from current structures (rejected:
the meeting place wandering with structure churn is confusing story-wise and adds
re-derivation edge cases); a new map landmark at genesis (rejected: violates the
no-starting-buildings cold-start principle).

## R4 — Convening mechanism (how villagers actually gather)

**Decision**: during convening/open phases the executor pins each living, awake,
non-exiled villager's intent to `Goal:"attend_meeting"` targeting the meeting place
(source `"meeting"`), overriding current intents on the regular staggered beat;
arrivals wait (idle at the place). The mind's absorb loop sees `meeting.convened` and
suppresses planner scheduling for attendees until `meeting.closed` (the asleep-agent
precedent), so no wasted local-tier calls fight the pin. Asleep villagers sleep
through it; they simply miss the meeting.

**Rationale**: intent-pinning reuses the existing intent/movement/BFS machinery
untouched; suppressing the planner during meetings mirrors how sleep already gates
planning. No new goal enters the planner vocabulary — `attend_meeting` is
executor-set, never model-chosen, so `resolveGoal` needs no new case.

**Alternatives considered**: a planner-visible `attend_meeting` goal the model may
choose (rejected: attendance must not depend on model availability or whim — skipping
the meeting by *choice* is post-v1 characterization); teleporting attendees (rejected:
watching the village converge IS the feature).

## R5 — Speaking turns and the deterministic proposal floor

**Decision**: while the meeting is open, a turn beat fires every
`meetingTurnTicks = 360` (6 game-min): the next attendee in seating order (ascending
agent index — stable and legible) gets `meeting.turn_taken{agent, raised}`. On their
turn, fodder rules — pure functions of state — decide whether they table a proposal:

1. **Curfew** (`kind:"curfew"`, param `startSecond = nightStartSecond`): tabled by an
   agent holding a gru memory (attack/sighting) from the last 3 game days, if no
   active curfew norm exists.
2. **Repay-debts** (`kind:"repay_debts"`): tabled by a creditor of a broken debt if
   no active repay norm exists.
3. **Amend/Repeal**: an agent with ≥ `repealViolationCount (2)` recorded violations
   of an active norm tables — amend (curfew start +2h, once) if their affection
   toward the norm's proposer is ≥ 0, else repeal. Self-interest as character.
4. **Exile** (`kind:"exile"`, target): tabled when some living villager's mean
   (trust+affection) from all other living villagers is below
   `exileHostilityGate (−600)` and the proposer's own edge toward them is hostile.
   Rarest by construction — the valve of last resort.

First matching rule wins; at most one proposal per turn; agents with no fodder raise
their loudest grievance memory as the `raised` note (no proposal). Proposals identical
to an active norm are never tabled (the rules check first).

**Rationale**: tabling must be deterministic for SC-007 (degraded-mode governance)
and replay; fodder rules read exactly the social state the spec names as grievance
sources. Seating order beats reputation order (stable under mid-meeting reputation
changes).

**Alternatives considered**: model-decided tabling via injection (rejected: spec
assumption pins "voting is deterministic, speech is flavor" — outcome-shaping stays
mechanical); all-turns-then-votes agenda (rejected: needs open-proposal state across
beats; same-beat resolution is strictly simpler, see R6).

## R6 — Vote resolution (the relationship function)

**Decision**: a tabled proposal resolves in the same executor beat: for each attendee,
an integer score decides yea/nay —

- base: `Trust(voter→proposer) + Affection(voter→proposer)`
- exile proposals: `− (Trust(voter→target) + Affection(voter→target))` dominates,
  plus `base/4`; the target does not vote.
- amend/repeal: base, `+ selfInterestBonus (400)` if the voter has recorded
  violations of the norm in question.
- proposer always votes yea. Yea iff score ≥ 0.

Strict majority of eligible attendees passes; ties fail. One
`meeting.proposal_resolved` event carries proposal, per-voter positions, tally, and
outcome; its reducer enacts/amends/repeals the norm, applies pairwise voter edge
deltas internally (aligned +affection, opposed −trust, the `social.gave`
reducer-internal precedent), and appends outcome memories land as companion
`agent.memory_added` events in the same beat.

**Rationale**: "votes follow the relationship graph" made literal and replayable;
integer math on clamped edges, no floats, no RNG needed (score-0 default-yea gives
neutral strangers a status-quo-friendly but not obstructionist lean; the strict
majority + tie-fails rule carries the status-quo bias). Reducer-internal pairwise
deltas avoid O(N²) event spam while staying replay-identical.

**Alternatives considered**: seeded-RNG vote noise (rejected: legibility — the player
should be able to read WHY the vote went that way off the bonds); model-cast votes
(rejected: determinism contract); reputation-weighted vote power (rejected for v1:
one-villager-one-vote is legible; weighting is a post-v1 norm idea).

## R7 — Norm enforcement: violation detection

**Decision**: violations are detected only for the closed norm-kind vocabulary, all
deterministic, all requiring ≥1 witness within `witnessRadius`:

- **curfew**: piggybacks the per-game-minute needs beat — a non-exiled villager
  awake, outside any shelter/fire warmth radius after `startSecond` at night, with a
  witness, violates (cooldown-latched per night per agent, the near-death-latch
  pattern).
- **repay_debts**: piggybacks the existing hourly due-check — a `promise_broken`
  emitted while a repay norm is active also emits the norm violation (same beat).
- **exile**: per-game-minute — an exiled villager within `exileShunRadius` of the
  meeting place or any structure, with a witness, violates (latched per game-hour).

Each violation emits `norm.violated{norm, violator, witnesses}`; the reducer appends
to the norm's bounded violation record and moves witness→violator edges
(`normViolationTrust/Affection` penalties); companion events land a toned,
subject-tagged memory per witness — which is exactly the `TellableFor` gossip-seed
shape, so rumors spread organically with no new rumor machinery.

**Rationale**: FR-009 verbatim (witnessed-only, memories + edges + rumor fodder), and
every detector reuses an existing sweep beat — zero new scheduling machinery.
Unwitnessed breaches correctly cost nothing.

**Alternatives considered**: hard-enforcing compliance in the reflex policy
(rejected: FR-008 says agents may defy; the executor must not make crime impossible);
model-judged violations (rejected: determinism, and "the village only judges what it
can see" is a mechanical truth, not an opinion).

## R8 — Norms in planner context (obey / skirt / defy)

**Decision**: a "Village law" section in `userPrompt` (beside `socialContext`):
active norms rendered one line each ("The village voted (day 3, Birch's proposal):
everyone inside by nightfall."), standing exile judgments, and — during convening — a
"The village is gathering for the noon meeting" line. Reflex policy (`decideIntent`)
is deliberately norm-blind.

**Rationale**: FR-008 — norms constrain through *knowledge*, and the planner is where
knowledge meets persona. A defiant persona reading the curfew line and choosing to
forage at night is the story working as designed; reflex staying norm-blind means
degraded-mode agents can still violate norms (survival pressure), which keeps the
violation machinery observable even model-off.

**Alternatives considered**: norm-aware reflex (rejected: makes obedience mechanical
and uniform — the opposite of obey/skirt/defy); norms as injected memories (rejected:
law is standing state, not an episodic memory; memories age out of the window).

## R9 — The village charter file

**Decision**: authoritative norm state is event-sourced on `sim.State`; the charter
is `village_charter.md` at the world root, rendered by the scribe (dirty-marked on
governance events, rendered on start like souls) — provenance headers, rules in
force with proposer/day/tally, amendment history, repealed rules struck through, and
standing exile judgments.

**Rationale**: FR-006/FR-007 fall out by construction — a derived view of replayed
state is exactly as reconstructible and restart-safe as the state itself. The scribe
already owns "derived flat-file views of the replica" (souls, chronicle.md).
Naming: `charter.md` is Metatron-owned (player-editable, read-fresh); the village
file must not collide — `village_charter.md` + `world.VillageCharterPath()`.

**Alternatives considered**: an event-sourced-then-hand-editable file (rejected: two
write authorities corrupt determinism; the village's law is the sim's, the player
legislates through Metatron nudges); storing charter text only in the file (rejected:
FR-007 requires log-only reconstruction).

## R10 — Model enrichment: proposal phrasing

**Decision**: every tabled proposal carries deterministic template text (the floor,
always present). A small mind driver observes `meeting.proposal_tabled`, makes one
best-effort `llm.KindMeeting` (TierLocal) call to rephrase the template in the
proposer's voice, and injects `meeting.proposal_rephrased{proposal_id, text}` —
whitelisted, dry-run-validated (proposal must exist, text capped at
`normTextMax (280)`). Charter and chronicle prefer the rephrased text when present.
Failure or absence of the model changes nothing but flavor.

**Rationale**: FR-012's line — model as flavor whose results land as recorded
events — implemented with the smallest possible surface: one injectable event type,
one LLM kind, no outcome dependency. Local tier because phrasing is
planner-class volume/quality, and best-effort keeps it out of the conversation
priority lane's way.

**Alternatives considered**: phrasing synchronously inside the meeting beat
(rejected: the loop never waits on models); rephrasing votes/speeches too (rejected
for v1: one enrichment point proves the seam; more is tuning).

## R11 — Chronicle coverage

**Decision**: `chronicleNote` gains cases for `meeting.opened` (attendance),
`meeting.proposal_tabled` (proposer + text), `meeting.proposal_resolved` (tally +
outcome, voters named per TASK-17 convention), `norm.violated` (violator + witnesses),
and `meeting.closed`. Payloads all carry agent indices; `ChronicleEntryPayload.Agents`
is populated so entries mention participants.

**Rationale**: FR-013 / SC-008 — governance must be narratable story; the narrator
switch is the single choke point where new event families become visible.

**Alternatives considered**: none serious — omitting narrator cases silently hides
the feature from the player.
