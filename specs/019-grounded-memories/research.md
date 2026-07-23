# Phase 0 Research: Grounded Memories & Agent Journal

All decisions below are grounded against current `main` (9a25310; wiki notes verified at 6444c29) with a fresh file:line sweep. No NEEDS CLARIFICATION markers remained in the Technical Context; the research questions here are the design decisions the spec left to planning.

## R1 — Where memory context lives: extend `MemoryAddedPayload` + `Memory` with optional fields

**Decision**: Add optional, `omitempty` fields to `MemoryAddedPayload` (internal/sim/agents.go:688) and mirror them onto `Memory` (agents.go:126) in the reducer's `agent.memory_added` arm (state.go:319):

- `X, Y int` + `Place string` — location at emission (coords always for situated memories; `Place` a deterministic terrain/landmark description, may be empty). A boolean-presence problem exists for coords (0,0 is a valid tile): carry a `Where *MemoryPlace` pointer struct (`{X, Y int; Desc string}`) so absence is `nil`, not a fake origin.
- `Why string` — the driving intent's reason, verbatim; empty = absent, never fabricated.
- `Conv int64` — source-event ref for conversation memories (the founding-talk tick that keys every `social.conversation_turn`); 0 = absent.

**Rationale**: the payload IS the memory (event log is the only truth); enrichment must ride the event to stay reducer-applied and replay-safe. `omitempty` on every new field keeps pre-019 events unmarshaling to zero values and re-serialized snapshots byte-stable for old worlds (FR-014). A generalized `Refs []EventRef` was considered and rejected: the only ref class in scope is the conversation id, and `social.conversation_turn` events are keyed by `Conv` (social.go:138), not seq — a typed `Conv int64` matches the existing correlation scheme; a generic ref list is speculative structure with no second consumer.

**Alternatives considered**: (a) side-table events (`agent.memory_context`) joined at render — rejected: two events for one fact breaks atomicity and makes the scribe join state; (b) enriching only the text template — rejected: FR-001/FR-003 require *structured* context so consolidation/prompts can use fields, not re-parse prose.

## R2 — Getting "why" to the executor: `Intent` gains `Reason`, populated by the reducer from `agent.intent_set`

**Decision**: Add `Reason string json:"reason,omitempty"` to `Intent` (agents.go:49). The `agent.intent_set` event payload already carries `Reason` (loop.go:455/462) — the reducer's `agent.intent_set` arm copies it onto the stored intent. Executor memory emission sites (executor.go:671 switch) read `in.Reason` when baking the memory.

**Rationale**: today the reason's whole life is `InjectArgs.Reason` → one `agent.thought` (loop.go:559/582) → gone; the intent that executes minutes later has no memory of why. The event log already records the reason on `agent.intent_set`, so carrying it on reduced state adds zero new events and stays replay-identical (live and replay both populate it from the same event). Reflex intents set no reason → memories from reflex actions carry no `Why` — exactly the spec's "never fabricate" rule (FR-002, edge case 1). Plans: `set_plan` steps land through the same `InjectIntent` path; the plan-level reason applies to each step's intent as recorded by its `agent.intent_set`.

**Alternatives considered**: executor looks back through recent `agent.thought` events — rejected: out-of-band lookup at emission time, and thoughts aren't correlated to intents; threading through the in-memory `InjectArgs` only — rejected: replay would lose it (replay sees events, not InjectArgs).

## R3 — Place description: deterministic, from terrain the executor already sees

**Decision**: A pure helper `describePlace(s *State, x, y int) string` in sim — nearest notable feature by deterministic scan (same tile first: water/woods/forage/rock/den/station present; else "near <feature>" within a small fixed radius; else empty, coords suffice). Baked into the payload at emission; the scribe renders what's in the payload and never re-derives.

**Rationale**: terrain is regenerated from seed and stable (worldmap-generation); a pure function of (state, x, y) at emission tick is deterministic and replay-equal because it's baked into the event — replay never re-runs it. Keeping the helper trivial (fixed scan order, fixed radius) keeps it auditable.

**Alternatives considered**: named-landmark registry — no such concept exists yet; would be new world state for cosmetic gain. Render-time derivation — rejected: scribe would need map state and replay equality would hinge on a second implementation.

## R4 — Situated executor templates: extend the constructors, keep the salience table

**Decision**: Add context-carrying constructor variants in memory.go (e.g. `situatedMemoryEvent(tick, agent, salience, where *MemoryPlace, why string, format, args...)` and a `withConv` variant for the mind side) rather than widening every existing call. Executor call sites (executor.go:675–740, 374) migrate to the situated variants and their template strings gain where/why clauses composed deterministically: base text + optional " at/near <place> (x,y)" + optional " — <reason>" (exact grammar pinned in contracts/memory-context.md). Witness memories (`memoryAboutEvent`) gain the same where treatment (the witness's own location).

**Rationale**: constructors are the single choke point (all three build `MemoryAddedPayload` via `mustPayload`); adding variants preserves the untouched call sites' byte-stability while migrated sites change only where the feature demands it. Salience table unchanged — this feature situates memories, it does not re-weigh them.

## R5 — Conversation memory ref: `Conv` on the gist payload

**Decision**: convo.go:337's gist `MemoryAddedPayload` gains `Conv: cc.conv`. Transcript retrieval = filter event log for `social.conversation_turn` where `payload.conv == memory.Conv`, ordered by seq — already fully determined (social.go:138). The scribe renders the ref as a compact marker on the memory line.

**Rationale**: `cc.conv` is in scope at the emission site today and already keys every turn event; one field closes FR-005/SC-002 with no new events or indexes.

## R6 — Journal state shape: `Agent.Journal` as ordered entries with stable ids

**Decision**: `internal/sim/journal.go` defines `JournalEntry{ID int, Tick int64, Text string}` and `Agent.Journal Journal` (`Journal struct {NextID int; Entries []JournalEntry}`, `omitempty`). `journal.entry_written{agent, text}` appends with `ID = NextID` (then increments); `journal.entry_deleted{agent, entry}` removes the entry with that id (no-op with explicit gate rejection if absent). IDs are assigned by the reducer, monotonically, never reused.

**Rationale**: delete needs a stable address; entry ids assigned in the reducer are deterministic (pure function of event order) and survive replay. Text-match deletion was rejected (ambiguous under duplicate text; invites partial-string surgery on agent-authored prose). "Pages" were rejected: entries + one budget is the minimal structure that supports write/search/read/delete; any page abstraction is usage guidance in disguise, which the spec forbids.

## R7 — Budget enforcement: in the reducer, so the gate decides

**Decision**: `journalBudgetRunes = 4000` as a sim constant (memory-adjacent, journal.go). The reducer's `journal.entry_written` arm **returns an error** when `currentRunes + len([]rune(text)) > budget`. Because `InjectSocial` dry-runs every batch on a state copy before admitting it (loop.go:205), an over-budget write is rejected at the door — the handler translates that into `rejected_gate` with a reason naming the budget and current usage, which the tool-use loop feeds back to the model (toolloop verdict taxonomy). The journal is untouched; no event lands.

**Rationale**: this is Principle III applied to Layer 2 — the gate (reducer dry-run) decides, not handler courtesy; an over-budget write physically cannot become a fact. Constant-not-config: the budget participates in Apply's accept/reject decision, so it must be identical between live and replay; a config knob that can drift between runs of the same world would let a replay reject an event that landed live. A Go constant has exactly the same versioning semantics as every other deterministic sim constant (salience, durations). If per-world tuning is ever wanted, the budget belongs in the world manifest at genesis (recorded, immutable) — noted as future work, out of scope.

**Alternatives considered**: llm.json config knob (`Rounds()` precedent) — rejected for the replay-drift reason above; handler-side check only — rejected: a second writer path (future tool, bug) could overshoot silently, and doctrine says doors decide.

## R8 — Journal tools: two Expressive + two Read registry entries, villager-roster only

**Decision** (contracts/journal-tools.md pins the schemas):

| Tool | Effect | Gate | Lands/reads | Params |
|------|--------|------|-------------|--------|
| `write_journal_entry` | Expressive | dry-run budget | `journal.entry_written` | `text` (Text, capped `journalWriteCapRunes = 1000`) |
| `delete_from_journal` | Expressive | dry-run existence | `journal.entry_deleted` | `entry` (Number, min 0) |
| `search_journal` | Read | — | mind replica | `query` (Text, short cap) |
| `read_journal` | Read | — | mind replica | `entry` (Number, optional; absent = whole journal) |

All four join `LoopRosterVillager()` (roster.go:57). The two Expressive tools declare their `Events`, which `sim.ValidateToolCoverage` pins ⊆ `injectSocialWhitelist` at boot — the whitelist (loop.go:152) gains the two `journal.*` types. Write/delete are acting (consume the cognition's one action); search/read are Read-class and exempt — exactly FR-013 and the 017 cardinality rule, no loop changes needed. Metatron's roster is untouched (journals are private; the angel has no journal tools).

**Rationale**: this is the tool registry doing precisely what it was built for — capability as data, doors unchanged. The per-write text cap (1,000 runes, a `Cost.TextCapRunes` like `say`/`muse` carry) exists so a single call can't be larger than a quarter of the whole budget — it is a wire sanity cap consistent with every other expressive tool, not usage guidance; the budget proper remains the only journal rule. Search/read are the first production Read tools — spec 017 already lifted the roster restriction and specified Read dispatch semantics (`read_ok`/`read_error`, final-round `unlanded`), so the loop needs zero modification.

## R9 — Search/read semantics: deterministic, mind-replica-backed, private

**Decision**: handlers in internal/mind/handlers.go. `search_journal`: case-insensitive substring match of `query` over the acting agent's own `Journal.Entries` in the mind's replica; results returned newest-first as `read_ok` data (entry id, tick clock, text), bounded to a fixed result cap; empty result is a well-formed empty `read_ok`, not an error. `read_journal`: return the addressed entry, or the whole journal when `entry` absent; unknown id → `read_error` with reason. No cross-agent access path exists (handler reads only `cc`/job agent's state).

**Rationale**: substring search is deterministic, model-free, and honest about being simple — the experiment is about the agent's authoring behavior, not retrieval quality. Read results feed the transcript only (ephemeral, never replayed — tool-loop doctrine), so replica-freshness at dispatch is acceptable exactly as it is for every other read of the mind's worldview.

## R10 — Rendering: soul.md situated lines; new journal.md scribe view

**Decision**: scribe.go's memory line (scribe.go:244) grows deterministic suffixes from the reduced `Memory`: place ("— at <desc> (x,y)" / "— at (x,y)"), why ("— because <reason>"), conv ref ("[conv <id>]"). New `renderJournal(idx)` writes `agents/<name>/journal.md` (path via new `persona.JournalPath`) — header (name, budget usage "N/4000") + entries as `## <clock> (#id)` sections with the raw markdown text; regenerated on `journal.*` events (scribe run-loop switch, scribe.go:79, marking `dirty` per agent — journal events added alongside the `agent.*` arm). Genesis seeds an empty journal.md next to soul.md (files.go:44 precedent).

**Rationale**: scribe already owns every regenerable per-agent view and maintains its own replica — journal.md is `render()`'s exact pattern with a different body. Rendering the agent's raw markdown verbatim keeps the journal authentically agent-authored; the only scribe-added chrome is the entry header needed to show ids (the address delete/read use).

## R11 — Replay proof: extend the existing determinism suite

**Decision**: extend the replay/determinism suite with a fixture world exercising situated memories (build/forage/talk with reasons), a conversation, and journal writes/deletes (including a rejected over-budget write); assert (a) live-vs-replay `State` equality, (b) byte-identical soul.md and journal.md renders, (c) zero orchestrator calls during replay (existing harness invariant), (d) a pre-019 fixture log replays unchanged (FR-014/SC-007).

**Rationale**: SC-003 is the load-bearing invariant; the suite already proves it for every prior feature — this feature adds cases, not a new harness.

## Decision summary

| # | Decision | Closes |
|---|----------|--------|
| R1 | Optional `Where`/`Why`/`Conv` on payload + Memory | FR-001..003, 007, 014 |
| R2 | `Intent.Reason` populated from `agent.intent_set` | FR-002 |
| R3 | `describePlace` baked at emission | FR-001 |
| R4 | Situated constructor variants + template grammar | FR-004 |
| R5 | `Conv` on gist memories | FR-005 |
| R6 | `Agent.Journal` entries with reducer-assigned ids | FR-008, 011 |
| R7 | Budget in reducer; door rejects | FR-010 (SC-005) |
| R8 | 2 Expressive + 2 Read tools, whitelist + coverage gates | FR-009, 013 |
| R9 | Deterministic substring search, private, replica-backed | FR-012 |
| R10 | soul.md suffixes + journal.md scribe view | FR-006, 008 |
| R11 | Determinism suite extension | FR-007, 011, 014 (SC-003/007) |
