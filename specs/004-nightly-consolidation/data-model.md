# Data Model: Nightly Consolidation + Persona Firewall

## New state (internal/sim, snapshot-carried, reducer-owned)

### Belief

| Field | Type | Notes |
|---|---|---|
| ID | int64 | assigned by reducer from `State.NextBeliefID` (monotonic, like debts/rumors) |
| Statement | string | the conviction, in the agent's phrasing |
| Confidence | int | 0–100, clamped by reducer |
| Provenance | string | `witnessed` \| `told` \| `inferred` |
| Source | int | agent index for `told` (who told them); −1 otherwise |
| Subject | int | agent index the belief is about; −1 for world-beliefs |
| Tick | int64 | last revision tick |

Stored as `Agent.Beliefs []Belief` (append + in-place revision by ID). A revision with an
unknown ID is a no-op; a revision with ID 0 creates a new belief.

### Agent additions

| Field | Type | Notes |
|---|---|---|
| Beliefs | []Belief | see above |
| Narrative | string | current self-narrative, wholly replaced on accept |
| LastConsolidatedNight | int64 | game-day index of the last *judged* consolidation (accepted/rejected/skipped_empty); −1 genesis |
| ConsolidatedUpTo | int64 | tick high-water mark of digested memories; only advances on accept |
| LastConsolidateMark | int64 | tick of the last marker event (12-game-hour secondary guard) |

### State additions

| Field | Type | Notes |
|---|---|---|
| NextBeliefID | int64 | monotonic belief ID counter, starts 1 |

## Derived (not stored)

- **Episodic buffer** for agent A = `[m ∈ A.Memories : m.Tick > A.ConsolidatedUpTo]`,
  ordered by tick. Oversized buffers (> 60 entries) are truncated oldest-first *for the
  call input only* — state keeps everything.
- **Night index** of a tick = `tick / 86400` (game-day index; clock.secondsPerDay).

## Event payloads (internal/sim, all landed via the whitelisted injection door)

| Event type | Payload fields | Reducer effect |
|---|---|---|
| `agent.memory_promoted` | agent, mem_tick, text_hash, boost | first matching memory's Salience += boost (cap 10); no-op if absent |
| `agent.memory_faded` | agent, mem_tick, text_hash | first matching memory removed (forgotten); no-op if absent |
| `agent.belief_revised` | agent, belief_id (0 = new), statement, confidence, provenance, source, subject | create (assign NextBeliefID) or revise in place; clamp confidence |
| `agent.narrative_set` | agent, text | Narrative = text |
| `agent.consolidated` | agent, night, up_to, outcome, reason, promoted, faded, beliefs, cost_usd | bump LastConsolidatedNight + LastConsolidateMark; on `accepted` also ConsolidatedUpTo = up_to |

Day-gist reuses the existing `agent.memory_added` (Salience = SalConvoGist-class constant,
Subject = −1).

An accepted night's batch is ordered: promotes, fades, gist (`agent.memory_added`),
beliefs, narrative, marker — one atomic injection. A rejected/skipped night's batch is
the marker alone.

## Persona additions (internal/persona, authored constants — not state)

| Field | Type | Notes |
|---|---|---|
| Anchor | per-agent string | one authored temperament line; supplied in the prompt, must be echoed verbatim in output `nature` |
| DriftMarkers | per-agent []string | authored trait words that contradict the nature; any match in narrative/self-belief → reject |

## Invariants

1. persona.md write path unchanged: genesis-only, mode 0444; consolidation touches events
   only (FR-007).
2. All five new event types are reducer-total: any payload the whitelist admits applies
   without error on any state (no-ops where targets vanished) — replay safety (FR-005).
3. `LastConsolidatedNight` monotonically non-decreasing; at most one marker per agent per
   night index (FR-001).
4. `ConsolidatedUpTo ≤` current tick always; only `accepted` advances it, so rejected
   nights leave the buffer intact (FR-008).
