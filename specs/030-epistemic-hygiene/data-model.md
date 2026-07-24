# Data Model: Epistemic Hygiene for Emergent Lore

All shapes additive-`omitempty`; no format bump (research R7).

## Memory origin (`internal/sim/agents.go`, `memory.go`)

| Field | Type | Semantics |
|---|---|---|
| `Memory.Origin` | `string`, `json:"origin,omitempty"` | Closed vocabulary stamped at emission: `action` \| `witness` \| `report` \| `omen` \| `gist` \| `digest`. Absent (legacy) = unclassified. |

Direct perception = {`action`, `witness`, `omen`}. `report` (chest-owner any-distance memory) and `gist`/`digest`
are secondhand; absent is treated as secondhand (conservative). Stamp sites: the three situated constructors
(callers pass origin), the conversation-gist injection, the day-gist, the Metatron dream/omen delivery. The
`agent.memory_added` payload carries the same field verbatim (reducer copies, never derives).

## Belief (`internal/sim/consolidate.go`)

| Field | Type | Semantics |
|---|---|---|
| `Reinforced` | `int64`, `json:"reinforced,omitempty"` | Game tick of last direct-observation anchor: formation stamp, direct-evidence revision, or reinforcement event. 0 = legacy grandfather (no decay). |

Existing fields unchanged. `EffectiveConfidence(b Belief, tick int64) int` (new pure function beside the reducer):
`Confidence` if `Reinforced == 0`, else `round(Confidence × 0.5^(elapsedDays/BeliefHalfLifeDays))`.

Constants (doctrine, `internal/sim`): `BeliefHalfLifeDays = 8`, `BeliefConfidenceFloor = 20`. Rationale recorded on
TASK-79 (research R3): convictions decay an order of magnitude slower than memory vividness; the floor sits just
under the rumor tellability floor (25) so the story outlives the conviction.

## Consolidation contract (`internal/mind/consolidate.go`, `validate.go`)

Belief output object gains `"evidence": ["m3", "m7"]` — ordinal refs into the sent buffer, pre-trimmed to
`MaxBeliefEvidence = 4` best-first, resolved to durable `(tick, MemoryHash)` identities like promote/fade refs.

Validator provenance rule (deterministic, coerce-not-reject):

| Proposal | Evidence resolves to | Lands as |
|---|---|---|
| `witnessed` | ≥1 direct-perception memory | `witnessed` |
| `witnessed` | only secondhand memories | `told` (coerced, counted) |
| `witnessed` | nothing resolvable | `inferred` (coerced, counted) |
| `told` / `inferred` | anything | as proposed |

Coercion count rides the night's `agent.consolidated` marker telemetry (non-fatal).

## Events

**`agent.belief_revised`** (existing) — payload gains `evidence` (resolved identities: `[{tick, hash}]`) and
`direct bool` (≥1 cited memory is direct perception). Reducer: on formation, `Reinforced = e.Tick` if `direct`,
else 0→formation stamp per R3 (set `Reinforced = e.Tick` only when `direct`; a hearsay-only formation leaves the
clock at formation via the SAME stamp — see note). *Note (normative)*: formation always sets `Reinforced = e.Tick`
(the curve starts at formation for every new belief); subsequent revisions refresh it ONLY when `direct` (US2-AC3).

**`agent.belief_reinforced`** (new, whitelisted injection) — `{agent, belief_id}`; reducer sets
`Reinforced = e.Tick`; vanished target no-ops. The grounded-observation seam (no producer in 030).

## Read sites (effective confidence)

| Site | Behavior |
|---|---|
| scribe Beliefs section | ≥ floor: effective value + provenance as today; < floor: hedged form ("half-remembered: …"), no confidence number |
| consolidation held-beliefs block | effective values; < floor entries still listed (revisable) but marked faded |
| planner/other prompts surfacing beliefs | < floor: excluded |

## Eval artifacts (`specs/030-epistemic-hygiene/eval/`, TASK-73 template)

`old.md` / `new.md` (outcome-prompt variants) · `fixtures/` (scripted scenes: speculation, action-discussed-not-done,
controls) · `scripts/eval-prompt-79.sh` (repo `scripts/`, modeled on `eval-prompt-73.sh`) · `decision.md` (numbers +
ship/no-ship). Gist output shape unchanged.
