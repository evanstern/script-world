# Feature Specification: Tool Registry — single source of truth for agent capabilities (Layer 1)

**Feature Branch**: `014-tool-registry`

**Created**: 2026-07-22

**Status**: Draft

**Input**: User description: "Tool registry: single source of truth for agent capabilities (Layer 1). Formalize everything an agent can do — metatron to villagers — as a Tool: name + param schema + gate + effect + cost, in ONE registry. Core principle: a tool call is a REQUEST; an event is the FACT; the gate decides; the executor grounds work in time and space. The model never asserts outcomes. Standalone formalization layer — NO model-API change, behavior-identical, replay-identical. De-risks the tool-use loop (TASK-52) and journal tools (TASK-16)."

## The problem being solved

Everything an agent can do today is described in several hand-maintained places that
must agree but are not forced to. The planner's goal vocabulary lives in a prompt
string, in a mind-side validation map, and in a sim-side validation map for plan
steps; each expressive capability (speaking, musing, conversation summaries, the
metatron's nudges) has its own bespoke prompt contract, parser, and injection
plumbing. Adding one verb touches roughly seven sites in lockstep.

This has already failed in the shipped code: the two mind-side lists carry all 19
verbs, but the sim-side plan-step list still carries only the original 10 — the nine
verbs added by the resource economy (spec 012) are silently rejected when a mind
expresses them as multi-step plan steps, violating spec 012's FR-020. The
duplication is not a theoretical risk; it is a live defect (tracked separately on
the board).

This feature replaces the scattered definitions with one registry that every
derived surface — prompt vocabulary, mind-side parse validation, sim-door
validation — is generated from, and makes "what may this agent do" a data question
(roster membership) instead of a plumbing question. It is a formalization layer
only: no change to how models are called, no change to behavior (one documented
exception), no change to replay.

**Core principle** (preserved, now stated as architecture): a tool call is a
REQUEST; an event is the FACT; the gate decides; the executor grounds work in time
and space. The model never asserts outcomes.

## Clarifications

### Session 2026-07-22

- Q: Should the plan-step drift cure (TASK-55) land with this layer, or should the layer reproduce today's behavior exactly? → A: Cure it here — the derivation fixes it, recorded as the sole permitted behavioral delta (FR-012); TASK-55 closes with this task.
- Q: What migrates to registry tools in this layer beyond the board-AC set? → A: Nothing — world verbs + say + muse + conversation gist + metatron nudges only; governance rephrase, consolidation writes, chronicle entries, and cognition telemetry keep riding the preserved whitelist door unchanged.
- Q: When does TASK-53 implementation start relative to TASK-51 (spec 013, In Progress, owns vocabulary growth)? → A: Planning proceeds now; implementation branches from main only after TASK-51 merges, so the migration enumerates a stable vocabulary once.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - One place to define a capability (Priority: P1)

A developer adding, changing, or retiring an agent capability edits exactly one
registry entry (name, parameter schema, gate, effect, cost) plus the code that
executes it. The prompt vocabulary the models see, the mind-side validation of
model replies, and the sim-door validation of arriving actions are all derived from
that single entry — none of them can drift from it, because none of them is
hand-maintained anymore.

**Why this priority**: this is the core of the feature — the single source of truth
whose absence caused the shipped plan-step drift. Every other story builds on the
registry existing.

**Independent Test**: register a test-only tool in the registry (in a test build);
observe it appear in the derived prompt vocabulary, pass mind-side reply
validation, and be accepted at the sim door — with no edits to any other site.
Remove it; observe it vanish from all three surfaces. Grep confirms the old
duplicate maps no longer exist.

**Acceptance Scenarios**:

1. **Given** the registry defines the current vocabulary, **When** the planner
   prompt is built, **Then** its goal vocabulary text is derived from the registry
   and is byte-identical to the prompt text produced today.
2. **Given** a model reply naming a goal, **When** the mind validates it, **Then**
   acceptance is decided by registry lookup, and the accepted set is exactly the
   set accepted today.
3. **Given** a plan step arriving at the sim door, **When** it is validated,
   **Then** acceptance is decided by registry lookup (see the one documented
   behavioral delta in FR-012).
4. **Given** a new tool entry added to the registry in a test, **When** the three
   derived surfaces are rebuilt, **Then** all three reflect it with zero edits
   elsewhere.

---

### User Story 2 - Existing capabilities migrate unchanged (Priority: P2)

Every capability agents have today — the 19 world verbs, speaking in
conversations, musing, conversation gists, and the metatron's nudges — is expressed
as a registry entry carrying its existing parameter shape, gate, effect class, and
cost. Worlds behave exactly as before: same prompts, same validation outcomes, same
events, same replay.

**Why this priority**: the migration is what makes the registry real rather than
aspirational, and its safety property (behavior- and replay-identical) is what lets
this layer merge without semantic review of every capability.

**Independent Test**: replay existing recorded event logs from before the change
and confirm the reconstructed final state is identical. Run a live smoke world and
confirm prompts are byte-identical and the event stream shape is unchanged.

**Acceptance Scenarios**:

1. **Given** an event log recorded before this feature, **When** it is replayed on
   the new code, **Then** the reconstructed state is identical to what the old code
   produced.
2. **Given** a running world, **When** agents plan, speak, muse, gist, and the
   metatron nudges, **Then** every capability works exactly as before — same event
   types, same payload shapes, same costs (durations, charges, text caps).
3. **Given** the registry migration, **When** the codebase is searched for the old
   hand-maintained vocabulary definitions, **Then** none remain.

---

### User Story 3 - Capability is roster membership (Priority: P3)

Each kind of agent has a roster — the list of registry tools it may use. Villagers
hold the world tools plus the expressive tools; the metatron holds its converse and
nudge tools. An action outside an agent's roster is rejected at the door exactly
like an unknown action. Granting or revoking a capability (e.g., a future chief who
may propose laws) becomes a roster edit, not new plumbing.

**Why this priority**: rosters turn future capability asymmetry into data, but
today's agents all have fixed capability sets, so this delivers structure rather
than new behavior.

**Independent Test**: in tests, submit a villager action naming a metatron-only
tool and a metatron action naming a villager-only tool; both are rejected at the
door with a recorded, non-fatal rejection. Confirm villager and metatron rosters
are expressed as data listing registry names.

**Acceptance Scenarios**:

1. **Given** the villager roster, **When** a villager action names a tool not on
   it, **Then** the door rejects it and no event lands.
2. **Given** the metatron roster (converse, nudge dream, nudge omen), **When** the
   metatron attempts a world verb, **Then** the door rejects it and no event lands.
3. **Given** a roster naming a tool absent from the registry, **When** the system
   starts, **Then** it fails fast with a configuration error rather than running
   with an inconsistent capability surface.

---

### Edge Cases

- A model reply names a tool that exists in the registry but is not on the acting
  agent's roster: rejected at validation with the same non-fatal handling as an
  unknown goal today (recorded, retried per existing policy, never a crash).
- A model reply names a tool that does not exist at all: unchanged behavior —
  rejected at parse/validation as today.
- A roster references a missing registry name, or a registry entry is malformed
  (empty name, duplicate name, no effect class): fail fast at startup, never at
  tick time.
- Replay of old event logs: replay reads events, not tool definitions — registry
  and rosters are not consulted during replay, so old logs reproduce identical
  state by construction. This must be verified, not assumed.
- The read-only tool class exists in the registry's vocabulary of effect classes
  but has zero entries in this layer; nothing may reference it at runtime until the
  tool-use loop (TASK-52) activates it.
- Concurrent vocabulary growth: inventory/storage work (spec 013, in progress) may
  add verbs while this feature is in flight; the migration must enumerate the
  vocabulary as it exists at integration time, not as of this spec's writing.
- Musing cadence: muse becomes a registry entry (roster tool), but its scheduling
  and trigger are untouched in this layer — agents do not yet *choose* to muse;
  that arrives with TASK-52.

## Requirements *(mandatory)*

### Functional Requirements

**The registry**

- **FR-001**: There MUST be exactly one registry defining every agent capability as
  a tool with: a unique name, a parameter schema (what arguments it takes and
  their constraints), a gate (the precondition class checked against live state
  before anything lands), an effect class, and a cost (duration, charges, and/or
  text-size budget as applicable).
- **FR-002**: The registry MUST distinguish three effect classes: **world tools**
  (produce an intent that the executor grounds in time and space), **expressive
  tools** (produce an immediate, bounded batch of whitelisted event types), and
  **read tools** (return data into cognition, produce no events). Read tools are
  declared as a class but have no entries in this layer.
- **FR-003**: Registry contents MUST be validated at startup: unique non-empty
  names, well-formed schemas, a known effect class, and every roster entry
  resolving to a registry name. Violations fail fast before the world runs.

**Derived surfaces (the duplicate maps die)**

- **FR-004**: The goal vocabulary presented in planner prompts MUST be derived
  from the registry, and the derived prompt text MUST be byte-identical to the
  prompt text produced before this feature.
- **FR-005**: Mind-side validation of model replies (which goal names are
  accepted) MUST be derived from the registry and MUST accept exactly the set
  accepted today.
- **FR-006**: Sim-door validation of arriving intents and plan steps MUST be
  derived from the registry (subject to FR-012's single documented delta).
- **FR-007**: The previous hand-maintained vocabulary definitions (prompt string
  list, mind-side map, sim-side plan map) MUST be removed; no duplicate
  authoritative list of capabilities may remain anywhere.

**Rosters**

- **FR-008**: Capability MUST be expressed as roster membership: each agent kind
  has a roster listing registry tool names. The villager roster holds the world
  verbs plus the expressive tools (say, muse, gist); the metatron roster holds
  converse and its nudge forms (dream, omen). Rosters MUST be data, not code
  branches.
- **FR-009**: An action naming a tool outside the acting agent's roster MUST be
  rejected at the door — recorded and non-fatal, identical in handling to an
  unknown action today.

**Migration**

- **FR-010**: All existing capabilities MUST become registry entries: the world
  verbs as they exist at integration time (19 as of this writing: the original 10
  plus the 9 added by spec 012), say, muse, conversation gist, converse, and the
  metatron nudges. Each entry MUST carry the capability's existing parameter
  shape, gate, effect class, and cost unchanged (e.g., intent durations, metatron
  charges, musing and utterance text caps). Converse is a registry entry
  classified Expressive with an empty Events set (transcript-only; it lands no
  world event), consistent with FR-008's roster membership.
- **FR-011**: Behavior MUST be identical after migration: same prompts (FR-004),
  same validation outcomes (FR-005, FR-012), same event types and payload shapes,
  same costs. Replay of event logs recorded before this feature MUST reproduce
  identical state.
- **FR-012**: The sole permitted behavioral delta: deriving the sim-door plan-step
  vocabulary from the registry brings the nine spec-012 verbs into multi-step plan
  validation, curing the shipped drift defect (spec 012 FR-020 violation). This
  delta MUST be explicit in the change record; no other behavioral difference is
  permitted.

**Preserved boundaries**

- **FR-013**: Both existing injection doors — the intent door and the
  social-event whitelist door — MUST be preserved exactly. The registry formalizes
  what passes through them; it does not widen, narrow (beyond FR-012), or bypass
  them. Expressive tools MUST only be able to land event types the existing
  whitelist allows.
- **FR-014**: The landing ladder — generation checks, staleness checks, and
  guards — MUST be unchanged and covered by tests that would fail if the registry
  layer altered any rung.
- **FR-015**: There MUST be no change to how models are called: same prompt
  contracts, same reply formats, same parsing. This layer changes where
  capability definitions live, not how minds communicate.

### Key Entities

- **Tool**: one capability an agent can request — name, parameter schema, gate,
  effect class, cost. The unit of the registry.
- **Registry**: the single authoritative collection of all tools; the source from
  which every capability surface is derived.
- **Roster**: an agent kind's capability set, expressed as a list of registry tool
  names. Villager and metatron rosters exist in this layer.
- **Tool call (request)**: an agent's expressed wish to use a tool. Never a fact;
  it must survive gate and door validation to matter.
- **Event (fact)**: the recorded outcome that actually happened, landed through an
  existing door. The only currency of world state; the model never writes one
  directly.
- **Gate**: a tool's precondition, checked against live state at landing time —
  the decider between request and fact.
- **Cost**: what using a tool spends — time (intent duration), charges (metatron
  nudges), or text budget (utterance/musing caps).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Adding a new world verb requires touching the registry entry and its
  execution logic only — the count of hand-edited definition sites drops from ~7
  to at most 2, demonstrated by the test-only-tool exercise (US1).
- **SC-002**: 100% of pre-existing recorded event logs replay to identical state
  on the new code.
- **SC-003**: Planner prompts produced after migration are byte-identical to
  prompts produced before it, for identical world state.
- **SC-004**: Zero duplicate capability lists remain: a search of the codebase
  finds exactly one authoritative definition per tool.
- **SC-005**: 100% of out-of-roster and unknown-tool requests are rejected at the
  door with a recorded, non-fatal rejection; none lands an event.
- **SC-006**: The multi-step plan drift defect is cured: all verbs the prompt
  offers are accepted as plan steps (spec 012 FR-020 restored), and this is the
  only behavioral difference detectable between old and new code.

## Assumptions

- **Drift cure is in scope** (clarified 2026-07-22): the plan-step vocabulary
  drift is a defect (spec 012 FR-020 proves intent) and is cured by derivation,
  recorded as the single permitted behavioral delta (FR-012). TASK-55 tracks the
  defect and closes with this feature.
- **Migration scope** (clarified 2026-07-22): the capabilities migrating are
  exactly the world verbs + say + muse + conversation gist + metatron nudges (the
  board task's acceptance criteria). All other whitelist traffic — nightly
  consolidation writes, chronicle entries, governance rephrase
  (proposal re-texting), cognition telemetry — continues to ride the preserved
  whitelist door unchanged and is explicitly NOT re-expressed as registry tools
  in this layer.
- **Verb count is a snapshot**: 19 world verbs as of this writing; spec 013
  (inventory/storage, in progress) may add more before integration. The migration
  enumerates the vocabulary as it stands when implementation starts (see
  Dependencies: after TASK-51 merges).
- **Muse scheduling unchanged**: muse joins the registry and the villager roster,
  but its trigger/cadence mechanism is untouched; agent-chosen musing arrives with
  the tool-use loop (TASK-52).
- **Read tools are declared, not enabled**: the read-only effect class exists so
  TASK-52 can populate it, but no read tool is registered or callable in this
  layer.
- **No persistence format change**: registry and rosters are static definitions
  loaded at startup; nothing about them is written to event logs or snapshots, so
  replay compatibility is structural.
- **Rosters are per agent kind** (villager, metatron) in this layer; per-individual
  rosters (e.g., a chief) are a foreseen future roster edit, not built now.

## Dependencies

- **De-risks**: TASK-52 (agent tool-use loop) and TASK-16 (journal tools) build on
  this registry; neither is implemented here.
- **Sequencing** (clarified 2026-07-22): spec 013 / TASK-51 (inventory & storage)
  is In Progress and owns vocabulary growth. Planning for this feature proceeds
  now; implementation branches from main only after TASK-51's PR merges, so the
  vocabulary is enumerated once, against a stable main.
- **Cross-reference**: the plan-step drift defect is filed as its own board task;
  FR-012 records its cure here.
