# Data Model: Metatron v1

## World state (event-sourced, in `sim.State`)

### MetatronCharges

| Field | Type | Notes |
|---|---|---|
| `MetatronCharges` | `int` (`json:"metatron_charges"`, never omitempty — a spent-to-zero bank must round-trip as 0) | banked nudge charges, 0..3; genesis = 1 |

**Validation / invariants**
- Never < 0, never > 3 (reducer clamps; `InjectSocial` dry-run rejects a spend at 0).
- Changes ONLY via `metatron.charge_regenerated` (+1, cap 3) and `metatron.nudged` (−1, floor 0).
- Pre-TASK-12 snapshots lack the field → genesis default applies on unmarshal (documented upgrade behavior).

### State transitions

```
charges: 1 (genesis)
  ── executor crosses 6-game-hour boundary AND charges<3 ──▶ +1  (metatron.charge_regenerated)
  ── landed nudge ──▶ −1                                        (metatron.nudged)
```

Regeneration boundaries are absolute game-time multiples of 6h (ticks 21600, 43200, …),
independent of spend timing — a pure function of (state, tick), replay-identical.

## Event payloads (structs in `internal/sim/metatron.go`)

### `metatron.charge_regenerated`

| Field | Type | Notes |
|---|---|---|
| — | `{}` | tick on the event row carries the when; reducer: `charges = min(3, charges+1)` |

Emitted by the executor (never injected); excluded from the `InjectSocial` whitelist.

### `metatron.nudged`

| Field | Type | Notes |
|---|---|---|
| `form` | `string` | `"dream"` \| `"omen"` |
| `targets` | `[]int` | dream: exactly 1 living villager; omen: all living villagers at landing |
| `text` | `string` | Metatron's rendering, ≤ 400 chars — the ONLY text that reaches villagers |

Reducer: decrement charges (floor 0). Injected (whitelisted) as the head of an atomic
batch; the batch's `agent.memory_added` events carry the villager-facing memories.

### Companion memories (existing `agent.memory_added`)

One per target: `{Agent, Text: "<form prefix> + nudge text", Salience: salDream(=8), Subject: -1}`.
Form prefix: `"You dreamed: "` / `"You witnessed an omen: "` — provenance-unknown by
construction; interpretation is the villager's persona's job.

## Files (bound to the run, not event-sourced)

### `charter.md` (save-dir root)

- Seeded by `promptworld new` from the authored default persona; never overwritten after.
- ≤ 4,000 chars used (excess truncated with in-reply notice); missing → recreated from
  default + in-reply notice; empty → default used + notice.
- Read at the start of every Metatron turn and digest (this IS the edit-liveness mechanism).

### `metatron/soul.md`

- Starts empty (created at first component start).
- Appended: dated digest entries, moment lines (immediate, model-free), nudge records
  (form, target, judgment one-liner, charge balance).
- Prompt carries a bounded tail (newest N bytes/entries); the file itself is unbounded
  player-readable history.

### `metatron/transcript.md`

- Append-only console history (`> player` / `metatron:` turns, dated).
- Prompt carries the last few turns for conversational continuity across restarts.

## In-memory (component, `internal/metatron`)

| Entity | Fields | Notes |
|---|---|---|
| `Metatron` | replica `*sim.State`, orch, injector, worldDir, events chan, done chan, turnBusy (single-flight), digest buffer `[]string`, digestFrom tick, moment queue `[]string`, carry (failed-digest lines) | scribe/mind pattern; all model I/O off the absorb goroutine |
| `turnJob` | player text, charter, soul tail, transcript tail, status snapshot (charges, roster alive/dead, clock, queued moments) | immutable snapshot built by absorb-side; worker runs the call |
| `turnReply` (parsed) | `say string`, `nudge *{form, target, text}` | strict-JSON parse; unparseable → safe refusal, nothing lands |

## Relationships

```
player ──text──▶ Metatron prompt (charter + soul tail + transcript tail + status)
                     │ one KindMetatron cloud call
                     ▼
              turnReply{say, nudge?}
                     │ validate: form, target alive, text cap, charges ≥ 1
                     ▼ (if nudge)
   InjectSocial [ metatron.nudged + agent.memory_added × targets ]  ← atomic, whitelisted
                     ▼
   State: charges−1; villagers: provenance-unknown memories → persona interpretation
```

Moments: `agent.died` / `gru.attacked` / `social.promise_broken` → soul.md line +
queue → surfaced in next turn's `say`. Digests: 6-game-hour windows → soul.md entries.
Neither path can produce a nudge (structural: only a console turn builds a nudge batch).
