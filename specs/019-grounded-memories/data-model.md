# Data Model: Grounded Memories & Agent Journal

Grounded against internal/sim/agents.go, memory.go, state.go, social.go at main 9a25310. All new JSON fields are `omitempty` so pre-019 events/snapshots round-trip byte-identically (FR-014).

## 1. MemoryPlace (new)

```go
// MemoryPlace situates a memory: where the agent stood when it formed.
type MemoryPlace struct {
    X    int    `json:"x"`
    Y    int    `json:"y"`
    Desc string `json:"desc,omitempty"` // deterministic terrain/feature description; "" = coords only
}
```

Carried as a pointer (`*MemoryPlace`) wherever it appears: `nil` = pre-019 memory / no location (never a fake (0,0)).

## 2. MemoryAddedPayload (extended) — event `agent.memory_added`

```go
MemoryAddedPayload struct {
    Agent    int          `json:"agent"`
    Text     string       `json:"text"`
    Salience int          `json:"salience"`
    Subject  int          `json:"subject"`
    Tone     int          `json:"tone,omitempty"`
    Where    *MemoryPlace `json:"where,omitempty"` // NEW: location at emission
    Why      string       `json:"why,omitempty"`   // NEW: driving intent reason, verbatim; "" = none existed
    Conv     int64        `json:"conv,omitempty"`  // NEW: conversation ref (founding-talk tick); 0 = none
}
```

**Validation (emission-side, by construction)**: `Why` only ever copied from `Intent.Reason` (never synthesized); `Conv` only set by the conversation gist path; `Where` set by every executor/convo emission from the acting agent's position at the emission tick.

## 3. Memory (extended, reduced state)

```go
type Memory struct {
    Text     string       `json:"text"`
    Salience int          `json:"salience"`
    Tick     int64        `json:"tick"`
    Subject  int          `json:"subject"`
    Tone     int          `json:"tone,omitempty"`
    Where    *MemoryPlace `json:"where,omitempty"` // NEW, copied from payload
    Why      string       `json:"why,omitempty"`   // NEW, copied from payload
    Conv     int64        `json:"conv,omitempty"`  // NEW, copied from payload
}
```

Reducer arm (`state.go` `agent.memory_added`): appends `Memory{... , Where: p.Where, Why: p.Why, Conv: p.Conv}`. `Tick` still stamped from `e.Tick`. Generation-bump logic unchanged.

## 4. Intent (extended)

```go
type Intent struct {
    Goal      string `json:"goal"`
    // ... existing fields unchanged ...
    Reason string `json:"reason,omitempty"` // NEW: planner reason from agent.intent_set; "" for reflex
}
```

**State transition**: `agent.intent_set` reducer arm copies the payload's existing `Reason` onto the stored intent. Cleared with the intent on completion/abandonment (it lives on the intent, so it dies with it). Reflex-sourced intents carry `""`.

## 5. Journal & JournalEntry (new, reduced state)

```go
// Journal is one agent's self-authored notebook. Mutated ONLY by the two
// journal.* reducer arms; budget rule lives in Apply, not in any handler.
type Journal struct {
    NextID  int            `json:"next_id,omitempty"`
    Entries []JournalEntry `json:"entries,omitempty"`
}

type JournalEntry struct {
    ID   int    `json:"id"`
    Tick int64  `json:"tick"`
    Text string `json:"text"`
}
```

`Agent` gains `Journal *Journal json:"journal,omitempty"` — a POINTER (the `Hail`
precedent), not a value. `encoding/json`'s `omitempty` is a no-op on a non-pointer
struct, so a value `Journal` would always serialize `"journal":{}`; a pointer omits when
`nil`, so an agent that never journals stays byte-identical to a pre-019 snapshot (FR-014).
The reducer lazily allocates on the first write; scribe/search treat `nil` as empty.

**Invariants**:
- IDs reducer-assigned (`ID = NextID; NextID++`), monotonic, never reused — stable addresses for delete/read across the journal's whole life.
- Budget: `sum(len([]rune(e.Text)) for e in Entries) <= journalBudgetRunes (4000)` — enforced by the `journal.entry_written` Apply arm returning an error when a write would exceed it; the `InjectSocial` dry-run turns that error into a door rejection, so no over-budget event ever lands (SC-005).
- Entries ordered by ID (append order == event order == tick order).

## 6. Journal event payloads (new)

```go
JournalWrittenPayload struct {
    Agent int    `json:"agent"`
    Text  string `json:"text"` // agent-authored markdown, ≤ journalWriteCapRunes (1000)
}

JournalDeletedPayload struct {
    Agent int `json:"agent"`
    Entry int `json:"entry"` // entry ID to remove
}
```

**Event types**: `journal.entry_written`, `journal.entry_deleted` — both added to `injectSocialWhitelist` (loop.go); both reducer-applied (state.go → journal.go helpers); both declared as their tool's `Events` so `sim.ValidateToolCoverage` pins them at boot.

**Apply semantics**:
- `journal.entry_written`: budget check (error = reject) → append `JournalEntry{ID: NextID, Tick: e.Tick, Text: p.Text}` → `NextID++`.
- `journal.entry_deleted`: entry absent = error (reject — the door tells the agent nothing matched); present = remove, order of survivors preserved. Freed runes immediately available.

## 7. Constants (new, internal/sim/journal.go)

| Constant | Value | Meaning |
|----------|-------|---------|
| `journalBudgetRunes` | 4000 | total journal budget per agent (clarified 2026-07-22) |
| `journalWriteCapRunes` | 1000 | per-write wire cap (tool `Cost.TextCapRunes`) |
| `journalSearchResultCap` | 8 | max entries one search returns |

Sim constants, not config — the budget participates in Apply accept/reject, so it must be version-stable like every other deterministic constant (research R7).

## 8. Relationships

```text
agent.intent_set{Reason} ──reducer──▶ Intent.Reason ──executor bake──▶ MemoryAddedPayload.Why
Agent position at emission ─────────▶ MemoryAddedPayload.Where (+ describePlace desc)
convoCtx.conv ──────────────────────▶ MemoryAddedPayload.Conv ──keys──▶ social.conversation_turn.Conv (transcript)
journal.* events ──reducer──▶ Agent.Journal ──scribe──▶ agents/<name>/journal.md
                                     └──mind replica──▶ search_journal / read_journal results
Memory{Where,Why,Conv} ──scribe──▶ soul.md situated lines
```
