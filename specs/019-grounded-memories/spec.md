# Feature Specification: Grounded Memories — Situated Episodic Capture & Agent-Authored Journal

**Feature Branch**: `019-grounded-memories`

**Created**: 2026-07-22

**Status**: Draft

**Board task**: TASK-16

**Input**: User description: "Grounded memories: context-rich episodic capture plus agent-authored journal (TASK-16). Two layers: (1) enrich the deterministic episodic memory capture so every memory is situated — structured context (place/coords, driving intent reason when one exists, refs to underlying source events e.g. conversation id); situated executor memory texts (where + why, not bare verbs like 'Built a fire.'); conversation memories reference their transcript so what was said is retrievable; soul.md renders the structured context; all detail baked at emission and reducer-applied so replay reproduces identical souls with no model calls. (2) Agent-authored journal: each agent gets a personal markdown journal it writes itself via mind-exposed tools — write_journal_entry and search_journal core pair, read_journal / delete_from_journal curation; only imposed rule is a hard size cap enforced at write time, to observe emergent usage; journal mutations event-sourced and reducer-applied so replay reproduces identical journals."

## Clarifications

### Session 2026-07-22

- Q: When a journal write would exceed the size budget, what happens? → A: Reject the whole write — journal unchanged, agent informed of the budget; curation is the agent's problem.
- Q: Are read_journal / delete_from_journal in scope? → A: Yes — all four tools ship; cap without delete would make a full journal permanently read-only.
- Q: Journal size budget? → A: ~4,000 characters total per agent (single configurable default) — tight enough that curation pressure arrives within a few in-world days.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Situated deterministic memories (Priority: P1)

An observer reading an agent's soul (or the agent's own mind reading its memory window) sees memories that are situated: each episodic memory records where it happened, why the agent was doing it (when a driving intent reason exists), and what underlying happenings it refers to — instead of bare verbs like "Built a fire." A memory now reads like "Built a fire at the rock outcrop east of the den (23,41) — planned to keep the Gru at bay." The same detail is visible in the agent's soul document and available to the agent's future reasoning.

**Why this priority**: This is the foundation layer. Every downstream consumer — nightly consolidation digests, prompts, soul rendering, and the journal layer's usefulness — improves only if the memory stream itself is situated. Without it, the rest of the feature builds on impoverished input.

**Independent Test**: Run a world where an agent builds, forages, hunts, and moves. Inspect the emitted memory records and the rendered soul: every episodic memory carries a place, memories driven by a planner intent carry that intent's reason, and the memory text itself names where and (when known) why.

**Acceptance Scenarios**:

1. **Given** an agent executing a build intent whose planner supplied a reason, **When** the build completes and the memory is emitted, **Then** the memory record carries the location (coordinates and/or named place) and the driving reason, and the memory text expresses both.
2. **Given** an agent acting from reflex (no planner reason exists), **When** the memory is emitted, **Then** the memory carries the location and omits the reason field entirely (no fabricated cause), and the text is still situated by place.
3. **Given** a completed world run, **When** the soul document is rendered, **Then** each memory line displays its situated context (where, and why when present).

---

### User Story 2 - Conversation memories reference their transcript (Priority: P1)

When an agent remembers a conversation, the memory is not just a one-line gist — it carries a durable reference to the conversation it summarizes, so the full dialogue (every turn's actual words) is retrievable from the memory alone. An observer (or a future mind-context builder) can go from "Talked with Mira about the storm" to the exact words that were said.

**Why this priority**: Conversations are the richest social events in the world and currently the most lossy: the full turn text already exists as sibling events but memories never point at it. This is the same emission-time enrichment as Story 1 applied to the highest-value event class, and it directly upgrades what consolidation and prompts can draw on.

**Independent Test**: Run a world where two agents converse. Take any resulting conversation memory, follow its conversation reference against the event log, and verify the complete ordered transcript of that conversation is recovered — without any model call or out-of-band lookup.

**Acceptance Scenarios**:

1. **Given** a finished conversation between two agents, **When** each participant's conversation memory is emitted, **Then** the memory carries the conversation's identifier as a source-event reference.
2. **Given** a conversation memory with its reference, **When** the transcript is looked up from the event log, **Then** every turn of that conversation, in order, with speaker and full text, is retrievable.

---

### User Story 3 - Agent-authored journal with write and search (Priority: P2)

Each agent owns a personal markdown journal — a tiny wiki the agent writes itself. The agent's mind is given a write tool and a search tool (plus read and delete for curation); nothing tells the agent how or when to journal. The system's only imposed rule is a hard size budget enforced when the agent writes. What the agent chooses to note, and how it organizes or curates its journal under the budget, is emergent behavior the observer can study. Journal content the agent searches for comes back into its thinking context.

**Why this priority**: This is the second layer, meaningful only once the deterministic memory stream is situated (Stories 1–2). It is also the experimental payload of the feature — observing what self-authored memory behavior emerges — and depends on the existing agent tool-use loop for the search tool's results to enter context.

**Independent Test**: Run a world with journal tools enabled. Verify an agent can write an entry, later search for it and receive matching content back in its context, and that a write that would exceed the size budget is refused with the reason made visible to the agent.

**Acceptance Scenarios**:

1. **Given** an agent mind presented with the journal tools, **When** the agent invokes the write tool with entry text, **Then** the entry is durably recorded in that agent's journal and attributed to the tick it was written.
2. **Given** a journal containing prior entries, **When** the agent invokes the search tool with a query, **Then** matching journal content is returned into the agent's cognition context.
3. **Given** a journal at or near its size budget, **When** a write would exceed the budget, **Then** the write is rejected at write time, the journal is unchanged, and the agent is informed of the rejection and the budget.
4. **Given** a journal with entries, **When** the agent invokes the delete tool on some content, **Then** that content is removed and the freed budget becomes available for future writes.
5. **Given** any journal state, **When** no rule beyond the size budget exists, **Then** the system imposes no schedule, format, or content requirements on journal use.

---

### User Story 4 - Faithful replay of memories and journals (Priority: P1)

An operator replaying a world from its event log gets byte-identical souls and byte-identical journals — with zero model calls and zero out-of-band lookups. All memory context and all journal content is baked into events at emission time and applied through the same reduction path live and in replay.

**Why this priority**: Determinism is a load-bearing invariant of the whole substrate, not an optional property of this feature. Any enrichment that leaks emission-time-only knowledge (or requires a model at replay) would corrupt the event-sourcing contract. It is P1 because Stories 1–3 are only acceptable in forms that preserve it.

**Independent Test**: Run a live world producing situated memories and journal writes; replay the event log from scratch; diff the rendered souls and the journals — both identical, with the model layer verifiably never invoked during replay.

**Acceptance Scenarios**:

1. **Given** a completed live run with situated memories, **When** the event log is replayed, **Then** the rendered souls are identical to the live run's.
2. **Given** a completed live run with journal writes and deletes, **When** the event log is replayed, **Then** every agent's journal is identical to the live run's.
3. **Given** a replay in progress, **When** memories and journals are being rebuilt, **Then** no model calls and no lookups outside the event log occur.

---

### Edge Cases

- Memory emitted with no active planner intent (pure reflex action): place is still captured; reason is absent, never invented.
- Conversation memory for a conversation whose turns include an interrupted/abandoned ending: the reference still resolves to whatever turns were actually logged.
- Journal write that alone exceeds the entire budget (oversized single entry): rejected outright; agent informed.
- Search over an empty journal: returns an explicit empty result, not an error.
- Delete targeting content that does not exist in the journal: journal unchanged; agent informed nothing matched.
- Agent dies: its journal stops growing but remains part of world state (renderable/replayable) like its soul.
- Replay of an event log recorded before this feature: old memories without context render as before (no fabricated context); mixed-era logs replay cleanly.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Every episodic memory MUST capture the agent's location at emission — coordinates and, when the world knows one, a human-readable place description.
- **FR-002**: When the action producing a memory was driven by an intent that carries a reason, the memory MUST capture that reason verbatim; when no reason exists, the memory MUST omit the field rather than fabricate one.
- **FR-003**: A memory MUST be able to carry references to the underlying events it summarizes (at minimum: a conversation identifier for conversation memories), and those references MUST be resolvable against the event log alone.
- **FR-004**: Deterministic action memory texts MUST be situated — expressing where the action happened and why (when a reason exists) — replacing bare-verb phrasings; the enriched texts MUST remain fully deterministic (same inputs → same text).
- **FR-005**: Conversation memories MUST reference their conversation such that the complete ordered transcript (speaker + full text per turn) is retrievable from the memory's reference via the event log.
- **FR-006**: The rendered soul document MUST display each memory's situated context (place, and reason when present), and remain a faithful view over the stored memories.
- **FR-007**: All memory context MUST be baked into the emitted event and applied through the single state-reduction path, so live and replayed state agree; replay MUST require no model calls and no out-of-band lookups.
- **FR-008**: Each agent MUST have a personal markdown journal, part of durable world state, surviving restarts and reproducible by replay.
- **FR-009**: The agent mind MUST be offered journal tools: write entry and search as the core pair, plus read and delete for curation. Search/read results MUST flow back into the agent's cognition context through the existing tool-use loop.
- **FR-010**: The journal MUST enforce a hard size budget at write time: a write that would exceed the budget is rejected, the journal is left unchanged, and the rejection (with the budget) is reported back to the agent. No other usage rules — no schedule, format, or content requirements — may be imposed.
- **FR-011**: Every journal mutation (write, delete) MUST be event-sourced and reducer-applied; replaying the event log MUST reproduce byte-identical journals with no model calls.
- **FR-012**: Journal search MUST be deterministic (same journal + same query → same results) and MUST NOT depend on any model.
- **FR-013**: Journal write/delete MUST occupy the same "acting tool" budget as other world-affecting tools in a cognition, while search/read MUST be usable without consuming it — consistent with the established tool-class doctrine.
- **FR-014**: Event logs recorded before this feature MUST continue to replay correctly: memories without situated context render as they did before, without fabricated context.

### Key Entities

- **Memory (episodic record)**: An agent's remembered episode — text, tick, and now structured context: place (coordinates + optional description), driving reason (optional), and source-event references (optional, e.g. a conversation identifier).
- **Memory context**: The structured situating block baked into a memory at emission — where, why, and refs. Absent parts are omitted, never invented.
- **Conversation transcript**: The ordered turns (speaker, full text) of one conversation, already durably logged; now reachable from the conversation memory via its reference.
- **Journal**: One agent's self-authored markdown notebook — part of world state, owned and written exclusively by that agent's mind, bounded by a fixed size budget.
- **Journal entry**: A single agent-authored write: markdown text attributed to the tick it was written.
- **Journal tools**: The mind-facing capabilities — write entry, search, read, delete — through which the journal is used; the only channel that mutates a journal.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of newly emitted episodic memories carry a location; 100% of memories produced from a reasoned intent carry that reason.
- **SC-002**: For every conversation memory in a run, the full ordered transcript is recoverable from the memory's reference alone — for 100% of conversations, using only the event log.
- **SC-003**: Replaying any world produced after this feature yields souls and journals byte-identical to the live run, with zero model invocations during replay.
- **SC-004**: An agent can write a journal entry and, in a later cognition, retrieve it by search — demonstrated end-to-end in a live run.
- **SC-005**: No journal ever exceeds its size budget: over-budget writes are rejected 100% of the time and leave the journal unchanged.
- **SC-006**: A reader of a rendered soul can answer "where did this happen?" for every memory and "why?" for every reasoned one, without consulting anything else.
- **SC-007**: Pre-existing world logs replay without error and render as before (no regression on historic worlds).

## Assumptions

- **Cap-overflow behavior**: an over-budget write is rejected whole (not truncated), with the rejection and budget reported to the agent — leaving curation decisions (e.g., deleting old content to make room) to the agent. Chosen because truncation would silently corrupt agent-authored text and hide the constraint the experiment wants agents to feel.
- **Curation tools in scope**: read and delete are implemented alongside write and search. The task marks them optional, but a hard cap with no delete would make a full journal permanently read-only, defeating the emergent-curation observation the feature exists for.
- **Size budget is a fixed configurable character budget** with one default value for all agents — 4,000 characters (clarified 2026-07-22). Pages/entries are not separately capped — only total size.
- **Search is plain deterministic text matching** (e.g., case-insensitive substring/keyword) over the agent's own journal — no embeddings, no model, no cross-agent visibility.
- **Journals are private to their agent**: no tool exposes one agent's journal to another agent. Observer/operator surfaces (rendering, debugging) may display journals, like souls.
- **Place description reuses the world's existing deterministic terrain/landmark knowledge**; where no landmark applies, coordinates alone satisfy the location requirement.
- **Historic (pre-feature) events are not backfilled**: enrichment applies from this feature forward; old memories render as they always did.
- **Dependencies**: the agent tool-use loop (TASK-52, merged) and the tool registry with tool classes (TASK-53, merged) exist and are the delivery vehicle for the journal tools.
