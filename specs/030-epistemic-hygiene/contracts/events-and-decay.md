# Contract: Belief Events, Decay Arithmetic, Read Sites

## `agent.belief_revised` (existing type, additive payload fields)

```json
{"agent": 3, "id": 7, "statement": "...", "confidence": 62, "provenance": "told",
 "source": 1, "subject": -1, "evidence": [{"tick": 108001, "hash": 123456789}], "direct": false}
```

- `evidence`: resolved durable identities of cited memories (post-validator). `direct`: whether ≥1 is
  direct-perception — derived BEFORE landing so replay never re-classifies.
- Reducer: formation (`id` new) sets `Reinforced = e.Tick` always (the curve starts at formation); revision of a
  held belief sets `Reinforced = e.Tick` iff `direct`, else leaves it. Both fields omitempty — pre-030 events
  replay to `Reinforced` untouched (0 = grandfathered).

## `agent.belief_reinforced` (new, whitelisted through `inject_social`)

```json
{"agent": 3, "belief_id": 7}
```

Reducer (total): named belief exists → `Reinforced = e.Tick`; vanished → no-op. This is the grounded-observation
seam (perception-of-absence task is the intended producer; 030 ships consumer + tests only).

## Decay (computed, never stored)

```
EffectiveConfidence(b, tick):
  if b.Reinforced == 0: return b.Confidence                  // legacy grandfather
  days = float(tick - b.Reinforced) / 86400
  return round(b.Confidence * 0.5^(days / BeliefHalfLifeDays))
```

`BeliefHalfLifeDays = 8`, `BeliefConfidenceFloor = 20` — doctrine constants in `internal/sim`, human-tuned only.
No decay events; no stored mutation; pure tick arithmetic (pause/speed-invariant); identical inputs → identical
output (FR-006).

## Read sites (FR-007)

- Scribe soul Beliefs: effective ≥ floor → rendered as today with the effective number; < floor → hedged line
  ("half-remembered: <statement>"), no number, grouped after live beliefs.
- Consolidation held-beliefs block: effective numbers; < floor marked "(faded)" but still listed with IDs
  (revisable).
- Any other prompt surface that lists beliefs: < floor excluded.

## Memory origin stamping

`agent.memory_added` payload + `Memory` gain `origin` (omitempty). Emission sites and their stamps: situated
personal constructors → `action`; situated about-event constructor → `witness`, EXCEPT the chest-owner
any-distance memory → `report`; Metatron dream/omen delivery → `omen`; conversation-gist injection → `gist`;
nightly day-gist → `digest`. Every emission site must stamp — a new unstamped site is a review-time error (the
constructors take origin as a required parameter, so the compiler enforces it).
