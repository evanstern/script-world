# Contract: Journal Tools & Events (Layer 2)

The mind-facing tool contract and the event/reducer contract behind it. Consumers: tool registry (declarations), tool-use loop (dispatch), mind handlers (implementation), sim door + reducer (gate), scribe (view).

## Tool declarations (internal/tool/registry.go)

| Tool | Effect | Params (kind) | Cost | Events | Roster |
|------|--------|---------------|------|--------|--------|
| `write_journal_entry` | Expressive | `text` (Text, required) | `TextCapRunes: 1000` | `["journal.entry_written"]` | villager loop |
| `delete_from_journal` | Expressive | `entry` (Number, required, min 0) | — | `["journal.entry_deleted"]` | villager loop |
| `search_journal` | Read | `query` (Text, required, short cap) | — | — (reads ground nothing) | villager loop |
| `read_journal` | Read | `entry` (Number, optional, min 0) | — | — | villager loop |

- All four join `LoopRosterVillager()` only. Metatron rosters unchanged (journals are private).
- `PromptGloss` per tool states WHAT the tool does and the budget number — no guidance on when/why/how to journal (FR-010's "no other rules" applies to prompts too). Suggested glosses pinned at implementation; reviewer checks for smuggled guidance.
- Params-derived `InputSchema` for all four (no `InputSchemaJSON` override needed — flat scalar surfaces).
- Boot gates: `tool.Validate()` passes; `sim.ValidateToolCoverage()` verifies both Expressive tools' `Events ⊆ injectSocialWhitelist`.

## Cardinality (unchanged 017 semantics — verified, not modified)

- `write_journal_entry` / `delete_from_journal` are acting: landing one consumes the cognition's single action; a second acting call in the same response → `rejected_cardinality`.
- `search_journal` / `read_journal` are Read: they never consume the action; on the final round they record `unlanded` without dispatch (017 rule).

## Handler contract (internal/mind/handlers.go)

- **write**: build `JournalWrittenPayload{Agent, Text}`, land via `InjectSocial` batch (with its `cog.outcome`, mirroring `handleMuse`). Door verdicts: dry-run budget failure → `rejected_gate`, reason `"journal budget: <used>/<budget> runes, entry needs <n>"` (fed back to the model verbatim). Sets `doorOutcome` per the existing convention.
- **delete**: build `JournalDeletedPayload{Agent, Entry}`; unknown id → door dry-run error → `rejected_gate`, reason `"no journal entry #<id>"`.
- **search**: `read_ok` with newest-first matches (max `journalSearchResultCap` = 8) as lines `#<id> <clock>: <text>`; zero matches → `read_ok` with an explicit "no matches" data string (never an error). Case-insensitive substring over the acting agent's own entries in the mind replica.
- **read**: with `entry` → that entry's full text, unknown id → `read_error`; without `entry` → the whole journal (ids + clocks + text).
- No handler may access another agent's journal — no parameter addresses an agent.

## Event & reducer contract

- `journal.entry_written{agent, text}` / `journal.entry_deleted{agent, entry}` — both on `injectSocialWhitelist`; mind-injectable only through the door; tick re-stamped and dry-run applied like every whitelisted batch.
- Apply arms (deterministic, model-free):
  - written: reject iff over budget; else append `{ID: NextID, Tick: e.Tick, Text}`, `NextID++`.
  - deleted: reject iff id absent; else remove entry, preserve order, ids never renumbered.
- Replay: identical event sequence ⇒ identical `Agent.Journal` bytes ⇒ identical journal.md (FR-011, SC-003).

## View contract (scribe → agents/<name>/journal.md)

```markdown
# <Name>'s journal

_<used>/<budget> runes_

## <clock> (#<id>)

<entry text verbatim>
```

- Regenerated on `journal.*` events (per-agent dirty mark); seeded empty at genesis alongside soul.md.
- Entry text rendered verbatim (agent-authored markdown is the artifact under study); scribe chrome is limited to the header lines above.

## The one rule (and the absence of others)

The ONLY journal constraint the system imposes is the 4,000-rune budget, enforced by the reducer at write time. Explicit non-rules, testable by their absence: no write cadence, no required format, no scribe normalization of entry text, no prompt guidance beyond tool existence + budget, no auto-pruning, no salience. (The 1,000-rune per-write cap is a wire sanity bound identical in kind to `say`/`muse` text caps, not a usage rule.)
