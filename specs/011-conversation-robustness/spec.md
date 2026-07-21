# Feature Specification: Conversation Robustness

**Feature Branch**: `011-conversation-robustness`

**Created**: 2026-07-21

**Status**: Draft

**Input**: User description: "Conversation robustness: tolerate one bad utterance and one bad outcome instead of abandoning the scene; probe MLX reasoning_effort under max_tokens=128. Board task: TASK-42. Two all-or-nothing failure sites in the conversation scene runner discard a whole multi-agent conversation scene on a single bad local-model reply: the utterance site (any failed/unusable say abandons the scene mid-dialogue) and the outcome site (after the transcript is fully generated, a malformed scene-summary reply discards every turn, memory, relationship change, and rumor transfer). Cross-cutting: persist the raw model reply on parse failure; harden the outcome prompt; probe MLX reasoning_effort under the utterance token cap."

## Context & Evidence *(live-world grounding)*

The conversation scene runner treats a scene as all-or-nothing at two points. Live
evidence from world `myworld-01` on 2026-07-21 (local model gemma4:12b-mlx, parallel 4):

- **Outcome-site loss**: 4 conversations lost their entire completed scene to a
  malformed summary reply (12:35 `'F'`, 13:06 `'H'`, 13:24 `'S'`, 13:30 `'H'` —
  in every case the offending character matches a participant's initial, indicating
  the model emitted the gist sentence as an unquoted value). Early loss rate ~18%
  of conversations, rising in absolute terms now that the parallel local tier lets
  more conversations run.
- **Blast radius**: an outcome failure discards every dialogue turn, the
  conversation record, all participant memories of the exchange, all relationship
  changes, and any staged rumor transfer — after ~75s of local compute produced a
  complete transcript. The villagers end the scene as if it never happened.
- **Utterance-site loss** (TASK-39): any single failed or unusable utterance
  abandons the scene mid-dialogue with the same total loss; one blank reply from a
  starved model kills a 13-point conversation.
- **Undebuggable failures**: the raw model reply is not persisted anywhere on
  parse failure — the failure mode above had to be inferred from the
  participant-initial correlation across occurrences.

Ruled out as causes: local-tier parallelism (first failure pre-dates it), the cloud
tier (scene calls never route there), and reply truncation (the malformed replies
are brace-balanced).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A completed scene survives one bad summary reply (Priority: P1)

A world operator runs a village where two agents hold a full conversation. The
model's final scene-summary reply is malformed. Instead of silently discarding the
entire scene, the system asks for the summary once more; on success, the scene
lands whole — turns, memories, relationship changes, rumors — exactly as if the
first summary attempt had succeeded.

**Why this priority**: this is the largest observed loss — the entire scene AND
all its social consequences are discarded at the last step, after all the compute
was already spent. Recovering it is pure win: the transcript already exists.

**Independent Test**: force a malformed first summary reply (fault injection or a
scripted fake model) and verify the scene lands intact after the retry; force two
consecutive malformed replies and verify the scene is abandoned with the same
observable telemetry as today.

**Acceptance Scenarios**:

1. **Given** a completed conversation transcript, **When** the first summary reply
   is unusable and the retry succeeds, **Then** the scene lands whole (turns,
   conversation record, memories, relationship deltas, rumor transfer) and
   telemetry records that a retry occurred.
2. **Given** a completed conversation transcript, **When** the summary reply is
   unusable twice in a row, **Then** the scene is abandoned exactly as today
   (no partial state) and telemetry records the double failure.
3. **Given** normal operation, **When** the first summary reply is valid,
   **Then** behavior is byte-for-byte identical to today (no extra calls).

---

### User Story 2 - A scene survives one bad utterance (Priority: P2)

Mid-dialogue, one agent's reply comes back empty or unusable. Instead of
abandoning the whole scene, the system tolerates the single bad reply — retrying
it once or skipping that speaker's turn — and the conversation continues. Two
consecutive bad replies may still abandon the scene.

**Why this priority**: same total-loss failure at a different point; less costly
than the outcome site (less compute is wasted mid-scene than at the end) but more
frequent under model starvation, and it blocks conversations from ever finishing.

**Independent Test**: inject one bad utterance mid-scene and verify the scene
completes and lands; inject two consecutive bad utterances and verify the scene
abandons with today's observable behavior.

**Acceptance Scenarios**:

1. **Given** a scene in progress, **When** exactly one utterance fails or is
   unusable, **Then** the scene continues (via one retry or a skipped turn) and
   eventually lands, with telemetry recording the recovery.
2. **Given** a scene in progress, **When** two consecutive utterances fail,
   **Then** the scene abandons as today, with no partial state injected.
3. **Given** a scene where every utterance is valid, **Then** the dialogue and
   its outcome are unchanged from today.

---

### User Story 3 - Parse failures are inspectable after the fact (Priority: P3)

When any scene reply (utterance or summary) fails to parse, the operator can
recover the exact text the model returned — from the world's durable record or
its log — without having to reproduce the failure.

**Why this priority**: the current failure mode was only diagnosable by
correlation across multiple occurrences because the offending text is discarded.
Every future robustness improvement depends on seeing what the model actually
said.

**Independent Test**: force a parse failure and verify the raw reply text is
recoverable verbatim from the world's records.

**Acceptance Scenarios**:

1. **Given** a parse failure at either site, **When** the operator inspects the
   world's records, **Then** the raw reply text is present and attributable to
   the specific conversation and call.

---

### User Story 4 - Fewer malformed summaries in the first place (Priority: P4)

The summary request itself is hardened so the model produces valid output more
often: the instruction makes explicit that free-text fields must be quoted
strings. Optionally, the parser tolerates the known unquoted-value shape and
recovers the fields without a second model call.

**Why this priority**: prevention lowers the retry rate and its added latency,
but the retry (P1) already bounds the damage; this is an optimization.

**Independent Test**: replay the observed malformed shapes against the hardened
parser/prompt and measure the recovery rate.

**Acceptance Scenarios**:

1. **Given** the hardened summary instruction, **When** scenes run on the local
   model, **Then** the malformed-summary rate is measurably lower than baseline.

---

### User Story 5 - The empty-utterance hypothesis is settled (Priority: P5)

The operator learns definitively whether the local MLX endpoint honors the
"no hidden reasoning" setting under the utterance token cap — i.e., whether a
thinking model is spending its whole utterance budget on hidden chain-of-thought
and returning empty text. Findings (positive or negative) are recorded on the
board task with the probe method.

**Why this priority**: it's a diagnosis that shapes future tuning, not a behavior
change; the tolerance work (P1/P2) is valuable regardless of the answer.

**Independent Test**: run the probe against the live endpoint at the utterance
token cap and record observed reply lengths/emptiness with and without the
setting.

**Acceptance Scenarios**:

1. **Given** the probe has run, **Then** the board task carries a recorded
   finding stating whether the setting is honored and what the empty-reply
   behavior is under the cap.

---

### Edge Cases

- Retry reply is also malformed → scene abandons; no partial state (turns without
  outcome, or outcome without turns) may ever land.
- The bad utterance is the scene's final turn → recovery must not produce a
  transcript the summary step can't handle (e.g., trailing skipped turn).
- Retry happens while the world is saturated (queue full / tier busy) → the retry
  follows the same admission rules as the original call; a refused retry counts
  as a failure, not a hang.
- Scene staleness: a retry adds wall-clock time — a scene that would land stale
  after the retry must still be subject to today's staleness handling (retry must
  not bypass the stale-at-landing check).
- Persisted raw replies may contain arbitrary model output — persistence must not
  break the durable record's encoding or size expectations (oversized replies are
  truncated with an indication).
- Multiple scenes running concurrently (parallel tier) each carry their own retry
  state; one scene's retry must not affect another's admission or ordering.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The scene runner MUST re-request the scene summary exactly once
  when the first summary reply fails to parse or validate; on retry success the
  scene MUST land whole, indistinguishable from a first-try success apart from
  telemetry.
- **FR-002**: The scene runner MUST tolerate exactly one failed or unusable
  utterance per scene (by single retry or by skipping that turn); a second
  consecutive failure MUST abandon the scene with today's semantics.
- **FR-003**: Scene abandonment MUST remain all-or-nothing: no partial scene
  state (turns, memories, relationship changes, rumors) may land when a scene is
  abandoned at either site.
- **FR-004**: On any scene-reply parse failure (utterance or summary, first try
  or retry), the raw reply text MUST be persisted in a durable, attributable form
  (bounded in size, with truncation indicated when applied).
- **FR-005**: Telemetry MUST distinguish first-try success, retry success, and
  double failure at each site, so recovery rates are measurable from the world's
  records.
- **FR-006**: The summary instruction MUST be hardened to state that free-text
  fields are quoted strings; the known unquoted-value failure shape SHOULD be
  recoverable by the parser without a model call.
- **FR-007**: Retries MUST follow the same admission, priority, staleness, and
  budget rules as the original calls — no new request kinds, no routing changes,
  no orchestrator changes, and no unbounded retry loops (at most one retry per
  site per scene).
- **FR-008**: The MLX reasoning-effort probe MUST be executed against the live
  local endpoint under the utterance token cap, and its findings MUST be recorded
  on the board task (the probe is an investigation deliverable, not a code
  behavior change).
- **FR-009**: When every reply parses on the first try, observable behavior MUST
  be identical to the current system (no additional calls, no added latency).

### Key Entities

- **Scene**: one multi-agent conversation from opening to landing — the dialogue
  turns plus the terminal summary that converts them into durable social state.
- **Utterance reply**: one speaker's generated dialogue turn; may be valid,
  unusable, or failed.
- **Summary (outcome) reply**: the terminal reply that condenses the scene into
  gist, topics, tones, relationship deltas, and rumor transfer.
- **Recovery record**: the telemetry + persisted raw text produced when a reply
  fails — the evidence trail for measuring and diagnosing robustness.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Scenes lost to a single malformed summary reply drop from the
  observed baseline (4 of ~22 conversations on the evidence world) to zero in an
  equivalent-length run; only double failures may still lose a scene.
- **SC-002**: Scenes lost to a single bad utterance drop to zero in
  fault-injection tests; two consecutive failures still abandon.
- **SC-003**: For every parse failure in a test run, the operator can produce the
  verbatim model reply from the world's records within one query/command.
- **SC-004**: With no injected faults, a scripted scene run produces identical
  durable output to the pre-change system (golden comparison), demonstrating
  zero behavior change on the happy path.
- **SC-005**: The board task carries recorded MLX probe findings and a
  measured before/after malformed-summary rate for the hardened instruction.

## Assumptions

- One retry per site is sufficient to capture most recoverable failures
  (observed failures are stochastic formatting slips, not deterministic model
  inability); more aggressive recovery is out of scope.
- Skipping a turn versus retrying an utterance is an implementation choice —
  either satisfies FR-002 so long as the scene completes and the transcript
  remains coherent for the summary step.
- The existing staleness, admission, and budget machinery is correct and
  unchanged; this feature only adds bounded tolerance inside the scene runner.
- Persisting raw replies on failure only (not on success) is sufficient for
  diagnosability and avoids bloating the durable record.
- The probe (FR-008) may conclude either way; its value is the recorded answer,
  and no code change in this feature depends on its result.
- Out of scope: per-class/provider routing (TASK-35), new LLM request kinds,
  orchestrator/queue changes, cloud-tier behavior, and conversation content
  quality beyond format validity.
