# Feature Specification: Norms and Votes

**Feature Branch**: `task-13-norms-and-votes`

**Created**: 2026-07-20

**Status**: Draft

**Input**: User description: "Norms and votes — village self-governance (TASK-13). The village legislates itself: agents propose rules (norms), votes resolve via the relationship graph, and passed norms become world constraints agents obey, skirt, or defy. Governance happens at a daily village meeting, not ad hoc: a coordination mechanism convenes the villagers — they break from their routines and physically gather at a meeting place so votes happen together, not scattered. The meeting runs once per game-day at noon, timeboxed to ~1 game-hour with grace to let an in-flight conversation finish; each villager gets a chance to speak (raise issues, propose new rules, propose amending or removing existing rules). Agreed rules persist in a village charter bound to the run's save directory; rules can be amended or removed via vote and the charter reflects the change. Passed norms constrain agent behavior: agents know the norms (they enter planner context), and may obey, skirt, or defy them — norm violations should be observable social fodder (memories/rumors/relationship consequences). This is also the substrate for possible exile-by-vote, the miscast valve of last resort. Grounding: docs/design/grounded-assumptions.md (The world), docs/wiki/social-fabric.md (relationship edges are the substrate votes follow), existing deterministic executor + event-sourced sim core."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The village convenes at noon (Priority: P1)

Once per game day, at noon, the village holds its meeting. As the hour approaches,
villagers break from whatever routine they are in — foraging, building, wandering —
and walk to the village meeting place, so that when the meeting opens they are
physically together. The meeting is timeboxed to about one game hour, with grace to
let an in-flight conversation finish rather than cutting a scene mid-sentence. Each
attending villager gets a chance to speak: raise an issue, propose a new rule, or
propose amending or removing an existing one. When the meeting closes, everyone
returns to their lives.

**Why this priority**: the meeting is the venue every other story runs inside —
without a reliable convening mechanism, proposals and votes happen scattered or not
at all. Governance-as-a-place is the feature's spine.

**Independent Test**: run the sim across a noon boundary and observe villagers
converge on the meeting place before/at noon, a meeting-opened event, per-villager
speaking turns, and a meeting-closed event within the timebox (+grace); afterwards
villagers resume normal behavior.

**Acceptance Scenarios**:

1. **Given** a running world approaching noon, **When** the convening window opens,
   **Then** awake villagers abandon their current routine and path toward the
   meeting place, and the meeting opens at noon with those present.
2. **Given** an open meeting, **Then** each attending villager gets a speaking
   opportunity before the meeting closes, recorded as events.
3. **Given** an open meeting reaching its one-game-hour timebox, **When** a
   conversation or vote is still in flight, **Then** the meeting grants a bounded
   grace period to let it finish, then closes; it never runs unbounded.
4. **Given** a closed meeting, **Then** villagers disperse back to their needs and
   routines, and no second meeting occurs that game day.

---

### User Story 2 - Propose, vote, pass (Priority: P1)

A villager with a grievance or an idea brings it to the meeting as a proposal: a
rule the village should live by ("no one takes from another's stores", "everyone
must be inside the palisade by dark"). The attending villagers vote, and each vote
follows the relationship graph — how much a villager trusts and likes the proposer
(and anyone the rule visibly targets) determines their yea or nay. A majority of
those present passes the rule; a tie or minority fails it. The outcome is announced
and remembered: villagers know what was decided and who stood where.

**Why this priority**: proposal → relationship-weighted vote → binding outcome is
the task's core acceptance criterion; it is what makes the village legislate
*itself* rather than follow authored rules.

**Independent Test**: seed a grievance (e.g., a broken debt), observe a related
proposal at the next meeting, verify each attendee's recorded vote matches the
deterministic relationship rule, and confirm the pass/fail outcome and its
announcement land as replayable events.

**Acceptance Scenarios**:

1. **Given** a villager with standing fodder (grievance, fear, or idea), **When**
   their speaking turn arrives, **Then** they can table a concrete proposal,
   recorded with proposer and text.
2. **Given** a tabled proposal, **Then** every attending villager casts exactly one
   vote, and each vote is a deterministic function of that villager's relationship
   edges (trust/affection toward the proposer and any target), so the same log
   replays to the same votes with no model calls.
3. **Given** votes cast, **Then** strict majority of attendees passes the proposal;
   ties and minorities fail it; the outcome event names the tally and each voter's
   position.
4. **Given** a passed proposal, **Then** villagers present remember the outcome
   (memory fodder), and how each villager voted is visible social information —
   votes move relationship edges between allies and opponents.

---

### User Story 3 - The charter remembers (Priority: P2)

The rules the village has agreed to live in a charter bound to the run's save
directory — a human-readable document the player can open, listing each rule in
force, when it passed, and on whose proposal. Amending or removing a rule happens
the same way it passed: a proposal and a vote at the meeting. When an amendment or
repeal passes, the charter reflects the change, and the history of what changed
survives in the world's event log.

**Why this priority**: persistence is what turns votes into law — without the
charter, passed norms evaporate and nothing binds tomorrow. It depends on Stories
1–2 existing first.

**Independent Test**: pass a rule, open the charter file and see it; pass an
amendment and a repeal across later meetings, re-open the charter and see the
amended text and the removed rule gone; replay the log and get an identical
charter.

**Acceptance Scenarios**:

1. **Given** a passed proposal, **Then** the charter in the run's save directory
   lists the rule with its provenance (proposer, day passed).
2. **Given** a passed amendment, **Then** the charter shows the amended rule text;
   **given** a passed repeal, **Then** the rule leaves the charter.
3. **Given** a replay of the event log alone, **Then** the reconstructed charter is
   identical — rules, text, provenance — with no model calls.
4. **Given** a daemon restart mid-run, **Then** the charter and the norms in force
   survive intact.

---

### User Story 4 - Norms bind (and get broken) (Priority: P2)

A rule in force is not decoration: villagers know the norms — the rules in force
enter their planning context — and each villager decides whether to obey, skirt, or
defy them, in character. When a villager violates a norm where others can see, the
violation is observable social fodder: witnesses remember it, it can become a rumor,
and it moves relationship edges against the violator. The village notices its laws
being broken.

**Why this priority**: constraint-with-teeth is what makes norms matter to the
story; but it needs rules to exist (Stories 1–3) before obedience or defiance means
anything.

**Independent Test**: pass a norm a specific villager's persona is inclined to
break, observe the norms appear in planner context, observe a witnessed violation
produce a violation event, witness memories, relationship movement against the
violator, and rumor-tellable fodder.

**Acceptance Scenarios**:

1. **Given** norms in force, **Then** every villager's planning context includes
   the rules in force, so obedience and defiance are informed choices.
2. **Given** a villager acting against a norm in force with at least one witness,
   **Then** a violation is recorded, witnesses gain a memory about the violator,
   and witness relationship edges move against the violator.
3. **Given** a witnessed violation, **Then** it is tellable gossip — it can seed a
   rumor about the violator that spreads like any rumor.
4. **Given** an unwitnessed violation, **Then** no social consequence lands
   automatically — the village only judges what it can see.

---

### User Story 5 - Exile is on the table (Priority: P3)

The gravest proposal a villager can table is exile: a vote to cast one of their own
out. It resolves like any other vote — relationships decide — but its subject is a
person, not a behavior. A passed exile enters the charter as a standing judgment:
the exiled villager is expelled from the community's protection, villagers shun
them, and the exile knows it. Whether the exile obeys (leaves, keeps to the edges)
or defies the judgment (stays, tests the village's resolve) is theirs to play out —
the miscast valve of last resort, socially enforced.

**Why this priority**: the grounding session parked exile as "possible via norms" —
an observation goal, not a headline feature. The substrate must exist; elaborate
banishment mechanics need not.

**Independent Test**: force relationships hostile enough toward one villager, table
an exile proposal, verify it resolves by the same vote rule, and confirm the passed
judgment lands in the charter, enters planning contexts, and witnesses treat
interaction with the exile per the norm system.

**Acceptance Scenarios**:

1. **Given** a meeting, **When** a villager tables an exile proposal naming another
   villager, **Then** it resolves by the same relationship-driven vote as any
   proposal (the subject does not vote on their own exile).
2. **Given** a passed exile, **Then** the judgment enters the charter naming the
   exile, and all villagers' planning contexts carry it.
3. **Given** a passed exile, **Then** the exile and the village both treat it as a
   norm in force — the exile obeying, skirting, or defying it is observable
   behavior with the same violation/witness consequences as any norm.

---

### Edge Cases

- **Nobody has anything to say**: the meeting still convenes at noon and closes
  early once every attendee has passed their turn; a quiet day is a short meeting.
- **Absent villagers**: asleep, distant, or otherwise unreachable villagers miss
  the meeting; votes resolve among attendees only, and absentees still learn passed
  rules through their planning context (the charter is public knowledge).
- **Death mid-governance**: a dead villager's open proposals die with them; a dead
  voter simply isn't an attendee; an exile vote naming a dead villager is moot.
- **Model unavailable (degraded mode)**: convening, speaking order, voting, and
  charter updates are deterministic and proceed without any model; only the
  *flavor* (proposal phrasing sourced from the mind) degrades to a deterministic
  floor. Governance never stalls on inference.
- **Timebox pressure**: if speaking turns plus votes would exceed the timebox, the
  meeting processes what fits (votes on tabled proposals take priority over new
  speeches) and closes; untabled business waits for tomorrow.
- **Duplicate/contradictory proposals**: a proposal identical to a rule in force is
  rejected at tabling; amendments and repeals must name the rule they modify.
- **The gru and the meeting**: the meeting is at noon precisely so night danger
  never collides with assembly; no special interaction is required.
- **Replay determinism**: every governance effect — convening, turns, proposals,
  votes, outcomes, charter changes, violations — lands as events; replaying the log
  reproduces the identical charter and relationship state with zero model calls.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST convene a village meeting once per game day at noon:
  a convening signal ahead of noon causes awake villagers to interrupt their
  routines and travel to a designated meeting place.
- **FR-002**: The meeting MUST open at noon with the villagers present, give each
  attendee a speaking opportunity (raise an issue, table a proposal to add, amend,
  or repeal a rule), and close within approximately one game hour, extended only by
  a bounded grace period for in-flight business.
- **FR-003**: An agent MUST be able to table a proposal (new rule, amendment,
  repeal, or exile) during their speaking turn; proposals record proposer, kind,
  text, and any target.
- **FR-004**: Votes MUST resolve deterministically from the relationship graph:
  each attendee's vote is a pure function of their trust/affection edges toward the
  proposer and any target villager, so outcomes are replayable without model calls.
- **FR-005**: A strict majority of attendees MUST pass a proposal; ties fail. The
  outcome, tally, and each voter's position MUST be recorded as events.
- **FR-006**: Passed rules MUST persist in a village charter bound to the run's
  save directory: human-readable, listing each rule in force with provenance
  (proposer, day passed, amendment history). Amendments and repeals passed by vote
  MUST be reflected in the charter.
- **FR-007**: The charter MUST be fully reconstructible from the event log alone
  (replay-safe) and MUST survive daemon restarts.
- **FR-008**: Rules in force MUST enter every villager's planning context so agents
  know the norms; agents MAY obey, skirt, or defy them — the system MUST NOT
  hard-enforce compliance of villager behavior.
- **FR-009**: A norm violation witnessed by at least one other villager MUST
  produce: a recorded violation event, a memory for each witness about the
  violator, relationship movement against the violator, and rumor-tellable fodder.
  Unwitnessed violations MUST produce no automatic social consequence.
- **FR-010**: Votes MUST be socially visible: how each villager voted is knowable
  by attendees and moves relationship edges between voters aligned and opposed.
- **FR-011**: Exile MUST be tableable as a proposal kind naming a villager; the
  subject does not vote; a passed exile enters the charter as a standing judgment
  and is treated as a norm in force by exile and village alike (socially enforced,
  not mechanically removed from the world).
- **FR-012**: All governance behavior (convening, meeting lifecycle, proposals,
  votes, charter changes, violations) MUST function with no language model
  available; model involvement is limited to enrichment (e.g., proposal phrasing)
  whose results land as recorded events.
- **FR-013**: The chronicle/event stream MUST carry governance events so meetings,
  votes, and violations are narratable story beats.

### Key Entities

- **Norm (rule)**: a village law in force — text, kind (behavioral rule or exile
  judgment), provenance (proposer, day passed), amendment history, active/repealed
  state.
- **Proposal**: a motion tabled at a meeting — proposer, kind (add / amend /
  repeal / exile), text, optional target (rule or villager), and lifecycle
  (tabled → voted → passed/failed).
- **Vote**: one attendee's position on one proposal — voter, proposal, yea/nay,
  derived deterministically from relationship edges.
- **Charter**: the persistent, human-readable document of rules in force, bound to
  the run's save directory; the authoritative public record the village (and
  player) reads.
- **Meeting**: the daily noon assembly — attendees, speaking turns, tabled
  proposals, outcomes, open/close times, timebox and grace.
- **Violation**: an observed breach of a norm in force — violator, norm,
  witnesses, and its social consequences (memories, edge movement, rumor fodder).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Across any multi-day run, a meeting convenes exactly once per game
  day at noon, and on a typical day at least 75% of living villagers attend.
- **SC-002**: Every meeting closes within its timebox plus grace (never more than
  25% over the one-game-hour target), and villagers resume routine behavior after.
- **SC-003**: Given a seeded grievance, a related proposal is tabled at a
  subsequent meeting and resolves to a recorded outcome within 2 game days.
- **SC-004**: The charter matches vote history exactly — after any sequence of
  passes, amendments, and repeals, 100% of rules in force (and none repealed)
  appear in the charter with correct provenance.
- **SC-005**: Replaying a run's event log reproduces the identical charter, vote
  tallies, and relationship state with zero model calls.
- **SC-006**: 100% of witnessed norm violations produce witness memories and
  relationship movement against the violator; 0% of unwitnessed violations do.
- **SC-007**: With the language model disabled for a full game day, the meeting
  still convenes, runs, and (given fodder) passes at least one rule — governance
  never stalls in degraded mode.
- **SC-008**: A viewer reading the chronicle can follow a rule's story — proposal,
  vote, passage, and any later violation — from narrated events alone.

## Assumptions

- **Voting is deterministic, speech is flavor**: "votes resolve via relationships"
  is read as a pure deterministic function of trust/affection edges (plus tie-break
  seeding already conventional in the sim); the language model may phrase proposals
  and speeches but never decides outcomes. This keeps replay model-free, matching
  the sim's event-sourcing invariant.
- **Attendee-majority, not census-majority**: votes resolve among villagers
  actually present at the meeting; absence is abstention. Strict majority passes;
  ties fail (status-quo bias).
- **Meeting place**: one designated gathering spot in the village area (chosen or
  established by the world; e.g., the fire/village center). No new construction
  mechanic is implied.
- **Proposal fodder floor**: with no model, proposals derive deterministically from
  standing grievances (e.g., broken debts, witnessed violations, gru attacks) via
  template rules — enough to prove governance end-to-end in degraded mode.
- **Exile is social, not mechanical**: a passed exile does not teleport or delete
  the villager; it is a charter judgment all agents know, enforced through the same
  obey/skirt/defy + witness-consequence machinery as any norm. Elaborate banishment
  mechanics (forced pathing off-map, death at the border) are out of scope for v1.
- **One meeting per day, noon only**: no emergency or ad hoc assemblies in v1;
  night danger (the gru) never interacts with governance by construction.
- **Scale**: the village is ~8 agents; speaking turns and votes comfortably fit
  a one-game-hour timebox at that scale, and no pagination/queueing of agenda
  beyond "tomorrow" is needed.
- **Dependencies**: relationship edges, memories, rumors, debts (TASK-8 social
  fabric), the planner context (TASK-7), the chronicle (TASK-11), and the
  event-sourced world substrate (TASK-2) all exist and are the surfaces this
  feature composes.
