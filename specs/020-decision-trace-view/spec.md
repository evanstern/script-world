# Feature Specification: Decision-Trace View

**Feature Branch**: `020-decision-trace-view`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "Decision-trace view: render the cog.tool_call verdict trail in the TUI (TASK-63, 'why did my agent do that'). The event log already persists a {verdict, reason} CallRecord as a cog.tool_call event for EVERY model tool call on every termination path, and cog.thought/cog.outcome bracket each cognition with agent, trigger_seq (the stimulus causality edge), and terminal outcome — but nothing renders the causal chain. This feature turns the already-persisted event stream into the prompt-engineering feedback surface a learner iterates against."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - "Why did my villager do that?" (Priority: P1)

A learner watching the world sees a villager do something surprising — or conspicuously
do nothing. They open that villager's detail view, switch to its **decisions** sub-view,
and read the causal chain of the villager's recent thoughts, most recent first: what
prompted the thought (a stimulus event, or the villager's own cadence), what the mind
tried (each tool call in order), what verdict each call received and why, and how the
thought ended (an action landed, or a terminal rejection/expiry). The learner uses this
to iterate on their prompt engineering: the trail shows exactly which requests the gates
refused and for what reason.

**Why this priority**: this is the whole point of the feature — the smallest change with
the biggest payoff for the teaching goal. The feedback signal already exists in the event
log; this story is the surface that makes it visible.

**Independent Test**: run a world until a villager has completed at least one cognition
with tool calls (including at least one rejection), open the villager's detail view,
switch to the decisions sub-view, and verify the chain reads stimulus → thought → tool
calls with verdicts → final outcome.

**Acceptance Scenarios**:

1. **Given** a villager whose recent cognition landed an action after one rejected tool
   call, **When** the viewer opens that villager's decisions sub-view, **Then** they see
   one chain showing the thought's class, both tool calls in emission order — the
   rejected one with a plain-language verdict and its reason, the landed one marked as
   the action that happened — and the thought's terminal outcome.
2. **Given** a villager with multiple completed cognitions, **When** the viewer opens the
   decisions sub-view, **Then** chains appear most-recent-first and the viewer can walk
   between them within the pane.
3. **Given** a cognition still in flight (thought recorded, no outcome yet), **When** the
   viewer opens the decisions sub-view, **Then** the chain renders honestly as in
   progress rather than pretending a terminal state.
4. **Given** a thought triggered by a stimulus event that the client has seen,
   **When** the chain renders, **Then** the stimulus line describes that event in the
   same readable voice as the chronicle; a cadence-triggered thought says so in plain
   words instead.

---

### User Story 2 - Metatron's own verdict trail (Priority: P2)

An operator conversing with Metatron in the TUI sees, inline in the transcript at the
turn where they occurred, the tool calls Metatron made and the verdict each received —
including refused miracles and rejected calls, with reasons — not just the successful
miracle lines the transcript shows today.

**Why this priority**: Metatron is the operator's own agent; its verdict trail is the
same teaching signal applied to the character the learner talks to directly. It reuses
the projection from Story 1 but renders in a different pane.

**Independent Test**: send Metatron a console message that causes at least one tool call
(e.g. a miracle request it refuses or grants), and verify the transcript shows a verdict
line per tool call, in order, at that turn.

**Acceptance Scenarios**:

1. **Given** a Metatron turn that made two tool calls (one landed, one gate-refused),
   **When** the transcript renders, **Then** each call appears inline with its
   plain-language verdict, and the refused one carries its reason.
2. **Given** a Metatron turn with no tool calls (a prose-only answer), **When** the
   transcript renders, **Then** no verdict lines are added.

---

### User Story 3 - Legible to a non-engineer (Priority: P3)

A non-technical reader (the learner the teaching game is for) reads any rejected tool
call in the decisions sub-view or metatron transcript and understands what happened
without knowing the verdict taxonomy: "the gate refused it because…", "the arguments
were malformed", "the villager had already used its one action this thought" — never raw
enum strings like `rejected_cardinality`.

**Why this priority**: without this, Stories 1–2 render a trail only an engineer can
read, which defeats the teaching purpose. It is cross-cutting polish over the other two
stories' surfaces.

**Independent Test**: force one of each rejection family (malformed, cardinality, gate)
and confirm each renders as a plain-language phrase with the underlying reason text, and
that no raw verdict enum appears anywhere in the two surfaces.

**Acceptance Scenarios**:

1. **Given** a tool call recorded with any verdict in the taxonomy (landed,
   rejected_gate, rejected_cardinality, rejected_unknown, rejected_malformed, read_ok,
   read_error, unlanded), **When** it renders in either surface, **Then** the verdict
   appears as a plain-language phrase and any recorded reason accompanies it.
2. **Given** a router-suppressed thought (an outcome with no thought and no calls),
   **When** it renders in the decisions sub-view, **Then** it reads as "didn't think
   because…" in plain words with the router's reason.

---

### Edge Cases

- **Chain fragments**: the client may connect mid-cognition — a tool_call or outcome can
  arrive whose thought was folded into the snapshot and never seen. Fragmentary chains
  render with what is known (the job's calls and outcome) rather than being dropped;
  attribution to a villager falls back to parsing the job identifier when the thought
  was missed.
- **Conversation cognitions**: conversation jobs are shared between two agents and carry
  no single-agent attribution; they are out of scope for the per-villager decisions view
  (documented, not silently dropped — see Assumptions).
- **Ring eviction**: the chronicle keeps only the most recent 500 events; decision
  chains must survive chronicle eviction. Conversely the projection itself is bounded —
  oldest chains per villager are discarded past the cap.
- **Reconnect**: after a disconnect/`dropped`-push teardown the client rebuilds its
  replica from a fresh snapshot; decision chains gathered before the teardown are lost
  and rebuild from the new subscription onward (consistent with the raw feed's existing
  behavior).
- **Oversized arguments**: recorded call arguments are already capped upstream (2 KiB,
  with a truncation marker); the view renders the capped form within the pane budget and
  never grows the projection unboundedly.
- **Dead villagers**: a dead villager's detail view still shows its decision chains —
  the trail of how it died is prime teaching material.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The client MUST maintain a per-villager decision-trace projection built
  incrementally as events are applied to the live replica, joining thought, tool-call,
  and outcome records that share a cognition job identifier — independent of the
  chronicle ring's 500-event cap.
- **FR-002**: The projection MUST be bounded: at most a fixed number of chains retained
  per villager (default 20, oldest evicted first), and per-chain content bounded by the
  upstream argument cap.
- **FR-003**: The villager detail view MUST offer a **decisions** sub-view, reachable
  and leavable by key from the detail view, rendering that villager's chains
  most-recent-first: stimulus line, thought class, each tool call in recorded order with
  tool name + verdict + reason, and the terminal outcome (or in-progress state).
- **FR-004**: The decisions sub-view MUST be walkable when content exceeds the pane
  budget (the viewer can move through chains within the pane), and MUST clip to the
  pane's row budget like every other panel body.
- **FR-005**: The stimulus line MUST render the triggering event in the chronicle's
  readable voice when the client has seen that event, MUST say plainly that the thought
  was cadence-driven when there is no trigger, and MUST degrade to a neutral reference
  when the trigger event is unknown to the client.
- **FR-006**: The metatron pane MUST render Metatron's own tool-call verdicts inline in
  the transcript, in call order, at the turn where they occurred, sourced from the same
  event stream (Metatron's job prefix), with the same plain-language treatment.
- **FR-007**: Every verdict in the taxonomy MUST render as a plain-language phrase in
  both surfaces; raw verdict enum strings MUST NOT appear. Recorded reasons MUST render
  alongside their verdict. Router-suppressed thoughts (outcome-only records) MUST render
  as suppressions with their reason.
- **FR-008**: Chain fragments (missing thought, missing outcome, or calls-only) MUST
  render honestly with what is known; an in-flight cognition MUST be visibly
  non-terminal.
- **FR-009**: The feature MUST NOT change the daemon, any event type, any payload, or
  the existing chronicle digest behavior; the existing event-catalog sweep test MUST
  still pass unchanged.

### Key Entities

- **Decision chain**: one cognition's causal record for display — stimulus reference,
  thought metadata (class, tick), an ordered list of tool-call records, and a terminal
  outcome (or its absence). Keyed by the cognition job identifier; attributed to one
  villager (or to Metatron).
- **Tool-call record (projected)**: one model tool call — ordinal, tool name, verdict,
  reason, capped arguments — as already persisted in the event log.
- **Stimulus reference**: the event-log sequence number that armed the thought's
  trigger, resolvable to a readable one-line description when the client saw that event.
- **Verdict glossary**: the fixed mapping from each verdict/outcome term to its
  plain-language phrase — the single authority both surfaces render from.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For any completed cognition that produced tool calls, a viewer can answer
  "what prompted it, what did it try, why did each attempt succeed or fail, and how did
  it end" entirely from the decisions sub-view — zero recourse to raw event payloads.
- **SC-002**: 100% of verdict taxonomy terms render as plain language in both surfaces;
  a sweep over the glossary proves no verdict falls through to its raw enum string.
- **SC-003**: Decision chains for a villager's last 20 cognitions remain viewable even
  after the chronicle ring has evicted the underlying events.
- **SC-004**: Every tool call in a Metatron turn produces exactly one inline verdict
  line in the transcript, in call order.
- **SC-005**: The client's memory for the projection stays bounded regardless of world
  age (per-villager chain cap × capped chain content).

## Assumptions

- The default retention of 20 chains per villager is enough to cover "recent behavior I
  am puzzled by" for the teaching loop; it is a display cap, not an analytics store.
- Conversation cognitions (shared two-agent scenes, job prefix `conversation-`) are
  excluded from the per-villager decisions view in this feature; their trail remains
  visible in the raw chronicle. A later feature may attribute them to both partners.
- Chains lost on reconnect are acceptable: the projection is a live observability
  surface over the subscription, matching how the raw feed already behaves; snapshots do
  not carry cognition history.
- Villager attribution when the thought record was missed relies on the existing
  villager job-identifier shape (`class-agentIndex-tick`); Metatron attribution relies
  on its existing `turn-metatron-` prefix. Both are stable, tested formats in the
  current codebase.
- The decisions sub-view lives inside the existing villager detail view and follows the
  established focus contract and pane-budget clipping rules; no new top-level tab is
  introduced.
