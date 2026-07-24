# Feature Specification: Epistemic Hygiene for Emergent Lore

**Feature Branch**: `task-79-epistemic-hygiene`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "Epistemic hygiene for emergent lore: honest belief provenance, hearsay decay,
attribution-preserving gists (TASK-79, from the 2026-07-23 world-01 Thornspire investigation). Villagers collectively
invented a place and phenomena that never existed — emergent mythology we WANT — but the epistemic machinery records
the fiction as fact. Three mechanisms, hygiene not suppression: provenance honesty (evidence refs + deterministic
validator enforcement), confidence decay with a reinforcement seam, attribution-preserving gists (eval-gated per the
TASK-73 precedent)."

**Doctrine context**: the Thornspire finding is the motivating specimen. A Metatron omen (tick 102060) was collectively
interpreted into an invented place and phenomena; 271 events now reference it. The invention is the point of the game —
villagers making myth out of mystery. The defect is that the epistemic bookkeeping *launders* the myth into fact: a
belief in never-observed tendrils recorded as provenance "witnessed" at confidence 68; conversation gists asserting an
investigation that never happened; nothing ever eroding unconfirmed conviction. This feature makes the bookkeeping
honest while leaving the mythology alive: **invention survives, as myth rather than fact.**

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Beliefs carry honest provenance (Priority: P1)

When a villager's nightly reflection distills the day into durable beliefs, each belief must now cite the memories it
rests on, and the label it carries must match what those memories actually are. "Witnessed" is reserved for beliefs
resting on at least one direct perception — something the villager did, or an omen/dream actually delivered to them. A
belief resting only on conversation and rumor is labeled as told or inferred, no matter what the reflecting mind
claims. The correction is quiet and mechanical: a mislabeled belief is kept but relabeled, never thrown away — the
same absorb-the-slack philosophy the nightly validator already applies to bookkeeping mistakes.

**Why this priority**: this is the root dishonesty (Birch's "witnessed" tendrils at confidence 68) and the mechanism
the other two stories build on — the evidence citations introduced here are what the decay clock (US2) reads to decide
whether a belief was ever directly confirmed.

**Independent Test**: drive scripted nightly reflections through the validator: one belief citing an own-action
memory keeps "witnessed"; an identical belief citing only a conversation-gist memory is coerced to hearsay; both land
and replay deterministically.

**Acceptance Scenarios**:

1. **Given** a nightly reflection proposing a belief labeled "witnessed" whose cited evidence includes a memory of the
   villager's own executed action or a delivered omen/dream, **When** the validator judges it, **Then** the belief
   lands with "witnessed" intact.
2. **Given** the same proposal where every cited evidence memory originated from conversation gists or rumor, **When**
   the validator judges it, **Then** the belief lands relabeled "told" (hearsay), the coercion is visible in the
   recorded outcome, and the night is NOT rejected for this alone.
3. **Given** a proposed belief citing no evidence at all, **When** the validator judges it, **Then** it can land only
   as "inferred" — never "witnessed".
4. **Given** any landed belief, **When** the operator inspects the event log, **Then** the belief's evidence
   citations resolve to durable memory identities, and each cited memory's direct-perception status is derivable from
   its recorded origin alone — no model involved in the classification.

---

### User Story 2 - Unconfirmed beliefs fade into myth (Priority: P2)

A belief that is never confirmed by direct observation loses conviction over game-days, the way an old memory loses
vividness. The stored record never changes — what decays is the *effective* confidence everyone reads: the villager's
soul document, their prompts, their nightly reflection input. Below a floor, the belief stops driving behavior (it no
longer surfaces in prompts) and renders in the soul as a half-remembered story rather than a conviction. A
reinforcement channel exists — an event any future system (the planned grounded-observation work) can emit to refresh
a belief's clock when the villager directly observes supporting evidence — fully specified and reducer-supported now,
even though nothing produces it yet.

**Why this priority**: decay is what turns laundered fact back into myth over time — Thornspire stays in the village's
stories, but nobody stakes decisions on tendrils no one has seen for a week. Depends on US1's evidence machinery for
the clock's honesty (a nightly revision resting only on hearsay must NOT refresh the clock, or nightly chatter keeps
myths eternally fresh).

**Independent Test**: fixture a belief formed on day 1 with no reinforcement; advance game time; effective confidence
follows the documented half-life curve deterministically; below the floor it disappears from prompts and renders
hedged in the soul; a reinforcement event resets the curve; replay of the whole run is byte-identical.

**Acceptance Scenarios**:

1. **Given** a belief never reinforced by direct observation, **When** N game-days pass, **Then** its effective
   confidence equals the stored confidence diminished by the documented half-life curve — computed identically on
   every read, with the stored value untouched.
2. **Given** a belief whose effective confidence falls below the floor, **When** prompts and the soul document are
   built, **Then** the belief no longer appears in prompts, and the soul renders it in an explicitly hedged form.
3. **Given** a nightly revision of a held belief whose cited evidence is hearsay only, **When** it lands, **Then**
   the stored confidence may change but the reinforcement clock does NOT refresh; **and Given** the revision cites
   direct-perception evidence, **Then** the clock refreshes.
4. **Given** a reinforcement event for a held belief (emitted by a test, standing in for the future
   grounded-observation channel), **When** it lands, **Then** the clock refreshes, the effective confidence returns
   to the stored value, and replay reproduces the identical state.
5. **Given** beliefs recorded before this feature (no reinforcement stamp), **When** a world loads and runs, **Then**
   they are grandfathered — no retroactive decay until a nightly revision or reinforcement first stamps them.

---

### User Story 3 - Gists preserve attribution (Priority: P3)

When a conversation ends and its gist is written into every participant's memory, speculation stays attributed and
history stays honest: "Rowan claimed he saw glowing tendrils" instead of "the team discussed the glowing tendrils",
and never "after investigating the tendrils" when no investigation occurred. This is a change to what the
conversation-summarizing prompt asks for, so it is **eval-gated per the TASK-73 precedent**: a scripted fixture set
measuring the before/after rate of fact-flattening and action-confabulation, plus a live sample, with the numbers
recorded on the board task — not vibes.

**Why this priority**: gists are the laundering pump — one flattened summary becomes identical salience-4 "facts" in
four minds at once. Independent of US1/US2 (pure prompt behavior), and the only story touching model-facing text.

**Independent Test**: run the eval fixture set (including a Thornspire-shaped scenario: one speaker speculates, no one
acts) against old and new prompts; the new prompt's gists attribute the claim and assert no unperformed action;
before/after numbers recorded.

**Acceptance Scenarios**:

1. **Given** a scripted conversation where one speaker makes an unverified claim, **When** the gist is produced under
   the new prompt, **Then** the claim is attributed to the speaker by name, not stated as shared fact.
2. **Given** a scripted conversation that discusses but does not perform an action, **When** the gist is produced,
   **Then** the gist does not assert the action happened.
3. **Given** the eval fixture set, **When** old and new prompts are compared, **Then** the confabulation/flattening
   rate is measurably reduced and no other gist-quality metric regresses beyond the recorded tolerance; the numbers
   land on the board task.
4. **Given** the live sample run, **When** its gists are inspected, **Then** no gist of the "after investigating"
   shape (asserting an unperformed action as done) appears.

---

### Edge Cases

- **Evidence citing a promoted/faded/vanished memory**: citations resolve against the reflection's own buffer (same
  mechanism as promote/fade references); a citation that no longer resolves is dropped from the belief's evidence and
  the provenance judgment uses what remains — an empty remainder falls to "inferred".
- **Omens about things that don't exist**: a delivered omen IS direct perception of the omen — a belief citing it may
  keep "witnessed" even though the omen's content is fiction. Content-grounding is explicitly the perception-of-absence
  task, not this one. The Thornspire myth therefore legitimately yields SOME witnessed beliefs ("we saw a rainbow
  sign") — but not tendril-beliefs evidenced only by conversation.
- **All-hearsay village**: a myth retold nightly never refreshes any clock (US2-AC3); it decays to the floor
  everywhere and lives on only in soul stories and rumor — myth achieved, suppression avoided.
- **Witness memories** (e.g. seeing a theft): direct perception — the witness stood there. Classification derives
  from the memory's recorded origin, not its text.
- **Decay at extreme speeds / pause**: decay is a pure function of game ticks, so speed and pause change nothing about
  its arithmetic; a paused world's beliefs do not decay (game time is frozen).
- **Legacy worlds**: pre-existing beliefs lack stamps and evidence; they are grandfathered (US2-AC5) and their
  provenance labels are left as recorded — history is not rewritten.

## Requirements *(mandatory)*

### Functional Requirements

**Provenance honesty (US1)**

- **FR-001**: Every belief proposed by a nightly reflection MUST carry evidence citations referencing memories from
  that reflection's buffer, resolved to durable memory identities on landing (the same reference discipline as
  promote/fade).
- **FR-002**: Each memory's direct-perception status MUST be deterministically derivable from its recorded origin:
  own executed-action memories and delivered omen/dream memories are direct perception; conversation-gist memories,
  rumor-seeded memories, and other secondhand memories are not. Witness memories of directly-seen events are direct
  perception. No model participates in this classification.
- **FR-003**: The deterministic validator MUST enforce: "witnessed" requires at least one direct-perception evidence
  citation; a "witnessed" proposal without one is coerced (not rejected) to "told" when it cites secondhand evidence,
  or "inferred" when it cites none; the coercion MUST be visible in recorded telemetry. Nights are never rejected
  solely for provenance coercion.
- **FR-004**: Tests MUST prove both directions: qualifying evidence preserves "witnessed"; non-qualifying evidence
  can never land "witnessed" regardless of what the model claims.

**Confidence decay and reinforcement (US2)**

- **FR-005**: Every belief MUST carry a reinforcement stamp (game-time). It is set at formation, refreshed by a
  nightly revision only when that revision cites direct-perception evidence (FR-001/FR-002), and refreshed by the
  reinforcement event (FR-008). Legacy beliefs without a stamp are grandfathered: excluded from decay until first
  stamped.
- **FR-006**: Effective confidence MUST be computed on read as the stored confidence diminished by a half-life curve
  over game-days since the reinforcement stamp — stored values never mutate from decay, no periodic decay events
  exist, and identical (belief, tick) inputs always yield the identical effective confidence. Constants (half-life,
  floor) are doctrine values versioned with the code; initial values and rationale MUST be recorded on the board task.
- **FR-007**: Below the floor, a belief MUST stop surfacing in any model-facing prompt and MUST render in the soul
  document in an explicitly hedged form (myth, not conviction). At or above the floor, rendering shows the effective
  (not stored) confidence.
- **FR-008**: A reinforcement event type MUST exist — whitelisted through the standard injection door, with a total
  reducer arm that refreshes the named belief's stamp (vanished targets no-op) — documented as the seam for the
  future grounded-observation channel, with tests exercising it even though no production producer exists yet.

**Gist attribution (US3)**

- **FR-009**: The conversation-summary prompt MUST instruct that unverified claims stay attributed to their speaker
  and that the gist never asserts an action occurred that no participant performed; the summary's downstream shape
  (memory per participant, tones, rumor paraphrase) is unchanged.
- **FR-010**: The prompt change MUST be eval-gated per the TASK-73 precedent: a scripted fixture set (including a
  Thornspire-shaped speculation scenario and an action-discussed-not-done scenario) measured before/after, plus a
  live sample; artifacts live under the spec's eval directory; the decision and numbers are recorded on the board
  task before the change ships.

**Determinism and compatibility**

- **FR-011**: Byte-identical replay MUST hold across all three mechanisms: validator coercion is deterministic,
  landed events are replayed not re-derived, decay is computed-on-read, and the reinforcement event is recorded
  input. Existing logs and snapshots MUST load and replay unchanged (additive shapes only; no format bump expected —
  justify against the spec-013 boundary if any shape question arises).
- **FR-012**: Provenance labels on already-landed beliefs are never rewritten; hygiene applies from this feature's
  deployment forward.

### Key Entities

- **Belief**: a villager's durable conviction — statement, stored confidence, provenance label, source/subject, and
  now: evidence citations (durable memory identities) and a reinforcement stamp (game-time). Effective confidence is
  derived, never stored.
- **Evidence citation**: a belief's link to a buffer memory, resolved to durable identity at landing; carries the
  memory's deterministically-derived direct-perception status.
- **Reinforcement event**: the recorded input that refreshes a belief's stamp — the seam for future grounded
  observation.
- **Gist**: the conversation summary written into each participant's memory; after this feature, attribution-preserving
  for unverified claims and honest about what was done versus discussed.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In a scripted reflection suite, 100% of beliefs evidenced only by conversation/rumor land as
  told/inferred; 0 land as "witnessed" (the Birch case cannot recur); beliefs with direct-perception evidence retain
  "witnessed" with 0 false coercions.
- **SC-002**: A never-reinforced belief's effective confidence follows the documented curve exactly (deterministic to
  the tick) and reaches the floor within the documented number of game-days; a reinforced belief resets exactly once
  per reinforcement.
- **SC-003**: Replay of a run containing coerced beliefs, decayed beliefs, and reinforcement events is byte-identical
  to the original.
- **SC-004**: On the eval fixture set, the rate of fact-flattened or action-confabulating gists drops by at least
  half versus the old prompt, with no regression beyond recorded tolerance on the other gist-quality checks; results
  recorded on TASK-79.
- **SC-005**: In a multi-game-day live sample with active conversation, at least one invented-lore thread remains
  present in souls/rumors (myth survives) while no belief about it above the confidence floor carries "witnessed"
  without direct-perception evidence (fact does not).

## Assumptions

- The nightly-reflection machinery (spec 004 + 019: buffer ordinals, durable memory identity, validator with
  absorb-mechanical-slack doctrine, atomic landing) is the substrate; this feature extends its contract rather than
  adding a new pipeline.
- The memory recency half-life and the rumor confidence-floor mechanics are the design precedents for decay constants;
  belief half-life is expected to be substantially longer than memory recency (convictions outlive vividness).
  Initial constants are hand-authored judgments recorded on the board task; retuning is human, from telemetry.
- Evidence citations extend the existing reflection output contract; the response-budget and cap philosophy of that
  contract (bounded lists, pre-trim then hard guards) applies to citations too.
- Rumor mechanics (birth, per-hop decay, tellability floor) are untouched; only belief-level state and the gist
  prompt change.
- The perception-of-absence work (separate task) will be the first real producer of reinforcement events; this
  feature ships the consumer side (seam + reducer + tests) only.
- Provenance vocabulary stays the existing three values; "hearsay" maps onto the existing "told" label rather than
  adding a fourth value.

## Amendments

- **2026-07-24 — US3 / FR-010 / SC-004 outcome**: the eval gate ran and was NOT met (eval/decision.md; bar
  pre-registered). The standard local tier (gemma4:12b-mlx) produces zero fact-flattened/confabulated gists with
  the CURRENT prompt (0/18, controls 12/12), so the attribution variant has nothing demonstrable to fix there;
  the tier that does exhibit the Thornspire failure (cogito:3b, world-01's configured local model) is not
  improved by the variant (3/18 → 5/18). Per the gate, the prompt does not ship and T011 is closed won't-ship.
  US3's end-state ("gists preserve attribution, no asserted-unperformed-actions") is instead evidenced on the
  standard tier by the eval baseline plus the T013 live sample; the failing-tier remediation is operational
  (upgrade world-01's local model), filed on the board.
