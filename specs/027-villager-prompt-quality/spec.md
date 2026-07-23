# Feature Specification: Villager Prompt Quality

**Feature Branch**: `task-73-villager-prompt-quality`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "Villager system-prompt quality pass (TASK-73). Rewrite systemPrompt in internal/mind/prompt.go for one clear identity statement (no five-fold name repetition), the persona block, tight task framing, and evaluate including one worked exemplar of good tool selection. Doctrine unchanged: acting-tool-only contract, muse-is-an-action framing, no free-text action path. Prompt stays the cacheable prefix (cache_control block boundaries preserved or consciously re-drawn). Behavior-affecting and eval-gated: before/after comparison on the scripted-stub suite AND a live soak (same seed, N game-hours) measuring rejected_malformed and rejected_cardinality rates, tool-selection distribution sanity, and prompt token count; ship only if rejection rates do not regress, and record eval numbers on the board task."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Villager decisions keep landing after the rewrite (Priority: P1)

The world operator runs the same world, same seed, before and after the prompt
rewrite. Villagers keep choosing valid actions at least as reliably as before:
the rate of model replies rejected for malformed arguments or for calling a
second acting tool does not go up, and the mix of actions villagers choose
stays sane (no collapse into a single verb, no musing flood).

**Why this priority**: this is the ship gate. The rewrite is behavior-affecting
and villager prompts run thousands of times per day on the local tier — a
quality regression here degrades every villager decision in every world.
Without this story proven, nothing ships.

**Independent Test**: run the before/after eval harness (scripted-stub suite +
live soak on a fixed seed for a fixed number of game-hours) against the old and
new prompt; compare rejected_malformed rate, rejected_cardinality rate, and the
acting-tool selection distribution.

**Acceptance Scenarios**:

1. **Given** the same world seed and soak duration, **When** the soak runs with
   the new prompt, **Then** the rejected_malformed and rejected_cardinality
   rates are no worse than the old prompt's rates on the same soak.
2. **Given** the soak's decision log, **When** acting-tool selections are
   tallied per variant, **Then** the new prompt's distribution shows no
   collapse (no single acting tool absorbing decisions that were spread before,
   no muse share explosion) by inspection of the recorded tallies.
3. **Given** the scripted-stub test suite, **When** it runs against the new
   prompt, **Then** all tests pass (updated only where they pinned the old
   prompt's literal wording).

---

### User Story 2 - The prompt reads as exemplary craft (Priority: P2)

A player (or developer) inspecting what a villager is told sees a prompt that
demonstrates the craft the game teaches: one clear identity statement, the
persona in its own block, and tight task framing — instead of the agent's name
repeated five times in one short paragraph.

**Why this priority**: this is the point of the task — a prompt-engineering
teaching game whose own core prompt is weak is ironic and undermines the
product. It is P2 only because shipping it is conditional on P1's gate.

**Independent Test**: render the system prompt for a named agent with a persona
and inspect it: the agent's name appears exactly once in the frame text
(persona text is free to use it), identity/persona/task-framing read as three
distinct parts, and the doctrine survives (exactly one acting tool per
decision, read-then-act order, musing framed as an action, no free-text action
path offered).

**Acceptance Scenarios**:

1. **Given** an agent name and persona, **When** the system prompt is rendered,
   **Then** the frame text (excluding persona content) contains the agent's
   name exactly once, in a single identity statement.
2. **Given** the rendered prompt, **When** its instructions are compared to
   doctrine, **Then** the acting-tool-only contract, the muse-is-an-action
   framing, and the absence of any free-text action path are all preserved.
3. **Given** two renders for the same agent (same name, same persona), **When**
   the outputs are compared, **Then** they are byte-identical — the prompt
   remains a stable, cacheable prefix.

---

### User Story 3 - The exemplar question is answered with evidence (Priority: P3)

The prompt author evaluates whether adding one worked exemplar of good tool
selection improves decision quality, and either ships the exemplar or records
the measured reason it was rejected.

**Why this priority**: the exemplar is the one open design question in the
rewrite; it must be resolved by measurement, not taste, but the rewrite is
valuable with or without it.

**Independent Test**: run the same eval (stub suite + soak) on the rewrite
with and without the exemplar; keep the better variant and record the numbers
and the decision on the board task.

**Acceptance Scenarios**:

1. **Given** two rewrite variants (with and without exemplar), **When** both
   are evaluated on the same seed and duration, **Then** the shipped variant is
   the one with the better (or equal, in which case cheaper-in-tokens)
   rejection rates, and the decision plus its numbers is recorded on TASK-73.

---

### Edge Cases

- Empty persona text: the prompt must still render as a coherent frame (no
  dangling blank block, no doubled newlines where the persona would be).
- The exemplar (if shipped) must not name a specific real agent or tool
  argument set that could leak into decisions as an anchor — villagers copying
  the exemplar's literal action instead of choosing situationally is a
  distribution-collapse failure caught by the soak's distribution check.
- Local-tier models are small (3B class): a longer prompt can degrade
  instruction-following even when rejection rates hold — token count is
  recorded so the cost of any added length is a conscious, recorded choice.
- Some scripted-stub tests may pin fragments of the old wording; they must be
  updated to the new wording without weakening what they actually test
  (doctrine, not phrasing).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The villager system prompt MUST open with a single identity
  statement that names the agent exactly once; the frame text MUST NOT repeat
  the agent's name elsewhere (persona text supplied per-agent is exempt).
- **FR-002**: The prompt MUST present the persona as its own distinct block,
  preserved verbatim from the agent's persona text, rendering cleanly when the
  persona is empty.
- **FR-003**: The prompt MUST preserve doctrine unchanged: the decision is made
  by calling exactly one acting tool; read-only tools may precede the acting
  call; musing is framed as an action (a beat spent thinking is a beat not
  spent doing); no free-text action path is offered or implied.
- **FR-004**: The rewrite MUST evaluate including one worked exemplar of good
  tool selection; the exemplar is shipped only if it does not worsen rejection
  rates, otherwise it is explicitly rejected with the measured reason recorded
  on the board task.
- **FR-005**: The system prompt MUST remain a stable function of the agent's
  name and persona (byte-identical across renders for the same inputs) so it
  stays a cacheable prefix; any change to what is included in that prefix MUST
  be a conscious, documented re-drawing of the boundary, not an accident.
- **FR-006**: Before/after evaluation MUST cover both (a) the scripted-stub
  suite and (b) a live soak on the same seed for the same fixed duration per
  variant, measuring: rejected_malformed rate, rejected_cardinality rate,
  acting-tool selection distribution, and system-prompt token count.
- **FR-007**: The rewrite ships ONLY if neither rejection rate regresses
  versus the old prompt on the same eval; the before/after numbers MUST be
  recorded on the board task (TASK-73).
- **FR-008**: Scripted-stub tests that pin prompt wording MUST be updated to
  the new prompt and the full suite MUST pass.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On the live soak (same seed, same duration), the new prompt's
  rejected_malformed and rejected_cardinality rates are each ≤ the old
  prompt's rates.
- **SC-002**: The rendered frame text contains the agent's name exactly once
  (down from five), verified by an automated test.
- **SC-003**: Acting-tool selection distribution on the soak shows no
  collapse: every acting tool selected under the old prompt at ≥5% share
  retains a nonzero share, and no single tool's share grows by more than 2×
  its old share, unless the deviation is explained and accepted in the
  recorded eval notes.
- **SC-004**: System-prompt token counts (old, new, and exemplar variant) are
  measured and recorded on TASK-73 alongside the rejection rates.
- **SC-005**: Same-input renders of the prompt are byte-identical (cacheable
  prefix preserved), verified by an automated test.

## Assumptions

- The scripted-stub suite means the existing deterministic tool-loop and mind
  test suites (stub model, no live provider); no new eval framework is built.
- The live soak uses the local tier's configured model on a freshly created
  world with a fixed seed, run for a fixed game-time window long enough to
  collect at least ~200 acting decisions per variant; the same seed, window,
  and provider are used for every variant compared.
- Rejection rates are computed from the world's durable decision log (tool-call
  verdicts recorded per decision), so no new telemetry is required to measure
  them.
- "Token count" is measured with the tokenizer/counting facility already
  available in the project (or a documented approximation applied identically
  to all variants); relative comparison matters more than absolute counts.
- Distribution sanity (SC-003) is a recorded-and-reviewed check with a stated
  numeric screen, not a hard automated gate, because action mix legitimately
  varies with world state.
- The Metatron half of this review item already shipped separately (TASK-64)
  and is out of scope; only the villager system prompt frame changes here.
- The user-prompt side (situation, needs, memories) is out of scope; only the
  system prefix is rewritten.
