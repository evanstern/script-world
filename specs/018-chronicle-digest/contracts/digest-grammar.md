# Contract: Chronicle Digest Grammar

**Feature**: specs/018-chronicle-digest | **Date**: 2026-07-22
**Consumers**: the TUI chronicle (dock tab, solo view, narrow fallback) and its tests. This contract is what the sweep test enforces and what `docs/design/tui/patterns/chronicle-grammar.md` is reconciled to.

## 1. Line format

```
solo:   <TICK> <HH:MM>  <type>            <summary>
dock:    <HH:MM> <short-type>  <summary>          (tick dropped; wraps ≤3 lines)
```

- Tick right-aligned to the widest visible tick; time width 5; type column padded to the widest visible type, cap 26 (solo) / short name (last `.` segment), cap 10 (dock).
- Solo: one line per event, overflow truncated `…`. Dock: wrap ≤3 lines then truncate (existing rules).
- `#seq` no longer appears on feed lines; it lives in the detail pane.
- Unknown/unregistered types render the current fallback: type + compact resolved-name JSON (never blank, never an error).

## 2. Voice by family (clarified FR-006)

| Family | Voice | Base tint applies to |
|---|---|---|
| agent, social, sim, world, governance (meeting/norm), gru, metatron, chronicle | natural phrase | type column |
| cog, clock, daemon | labeled fields (`key=value`, space-separated, stable order) | type column + summary |

Emphasis roles inside summaries: **name** (every resolved agent name), **speech** (quoted utterance), **emphasis** (amounts, item kinds, causes, outcomes, coordinates where load-bearing). High-salience types (`agent.died`, `gru.attacked`, `social.chest_taken`, `norm.violated`) render the whole line in the alert role.

## 3. Per-type digest templates

`{field}` = payload field; *Name* = resolved via `agentName`. Where a struct field is uncertain from the catalog, the implementer verifies against the payload struct and keeps the template's intent. Sample payloads for every row live in the sweep fixture (`digest_test.go`).

### world / clock / daemon

| Type | Template |
|---|---|
| `world.created` | `world "{name}" created · seed {seed}` |
| `world.migrated` | `migrated from format v{from_format} · {source_events} events @ tick {source_tick}` (state elided — detail pane bounds it) |
| `clock.paused` | `paused` |
| `clock.resumed` | `resumed` |
| `clock.speed_set` | `speed={speed}` |
| `clock.degraded` | `degraded rate={effective_rate}` |
| `clock.recovered` | `recovered` |
| `daemon.started` / `daemon.stopped` | labeled dump of payload fields (verified at impl) |

### sim

| Type | Template |
|---|---|
| `sim.day_started` | `day {day} begins` |
| `sim.night_started` | `night falls on day {day}` |
| `sim.forage_regrown` | `forage regrew at ({x},{y})` |
| `sim.fire_burned_out` | `the fire at ({x},{y}) burned out` |
| `sim.food_rotted` | `{n} {kind} rotted at ({x},{y})` |
| `sim.gathering_observed` | `gathering at ({x},{y}) since tick {start}`; all-zero payload → `gathering dispersed` |

### agent — acts & needs

| Type | Template |
|---|---|
| `agent.intent_set` | `{agent} intends {goal} ({source})` + target when present |
| `agent.work_started` | `{agent} set to work` |
| `agent.intent_done` | `{agent} finished` |
| `agent.intent_rejected` | `{agent}'s {goal} refused: {reason} ({staleness_ticks}t stale)` |
| `agent.moved` | `{agent} → ({x},{y})` |
| `agent.foraged` | `{agent} foraged at ({x},{y})` |
| `agent.chopped` | `{agent} chopped wood at ({x},{y})` |
| `agent.hunted` | `{agent} hunted at ({x},{y})` |
| `agent.quarried` | `{agent} quarried stone at ({x},{y})` |
| `agent.collected_water` | `{agent} drew water at ({x},{y})` |
| `agent.crafted` | `{agent} crafted {kind}` |
| `agent.built` | `{agent} built a {kind} at ({x},{y})` |
| `agent.dropped` | `{agent} dropped {n} {kind} at ({x},{y})` |
| `agent.picked_up` | `{agent} picked up {n} {kind} at ({x},{y})` |
| `agent.deposited` | `{agent} stored {n} {kind} in the chest at ({x},{y})` |
| `agent.withdrew` | `{agent} took {n} {kind} from {owner}'s chest` (owner = self → `their chest`) |
| `agent.cooked` | `{agent} cooked {produced} {kind} at the {station}` |
| `agent.bathed` | `{agent} bathed · morale {morale_after} warmth {warmth_after}` |
| `agent.refueled` | `{agent} refueled the fire at ({x},{y})` |
| `agent.spear_broke` | `{agent}'s spear broke` |
| `agent.ate` | `{agent} ate {consumed breakdown} → food {food_after}` |
| `agent.slept` | `{agent} fell asleep` |
| `agent.woke` | `{agent} woke` |
| `agent.needs_changed` | `{agent}` + labeled needs (`health=… food=… rest=… warmth=… morale=…` — verified: `NeedsPayload` has no water field) |
| `agent.died` | `{agent} died: {cause}` **(alert)** |
| `agent.talked` | `{a} chatted with {b}` |

### agent — mind & plans

| Type | Template |
|---|---|
| `agent.memory_added` | `{agent} remembers: "{text}"` (+ ` · about {subject}` when present) |
| `agent.thought` | `{agent} thought: "{text}" ({source})` |
| `agent.memory_promoted` | `{agent}'s memory (t{mem_tick}) reinforced` (verified: payload carries `text_hash`+`mem_tick`, never the text) |
| `agent.memory_faded` | `{agent} forgot a memory (t{mem_tick})` (same — no text in payload) |
| `agent.belief_revised` | `{agent} now believes: "{text}"` |
| `agent.narrative_set` | `{agent}'s story: "{text}"` |
| `agent.consolidated` | `{agent} consolidated the night's memories` |
| `agent.plan_set` | `{agent} planned {N} steps: {goals, comma-joined, truncating}` |
| `agent.plan_step_started` | `{agent} began step {step goal}` |
| `agent.plan_expired` | `{agent}'s plan lapsed ({reason})` |

### social

| Type | Template |
|---|---|
| `social.conversation_turn` | `{Speaker}→{Listener} "{text}"` (speech privilege preserved) |
| `social.rumor_told` | `{From}→{To} rumor: "{text}"` |
| `social.conversation` | `"{gist}" · {turns} turns` (tones elided to detail) |
| `social.relation_changed` | `{a}→{b} trust{±}/affection{±} ({reason})` (verified: two deltas, `trust_delta` + `affection_delta`) |
| `social.gave` | `{from} gave {to} {kind}` (verified: `GavePayload` has no amount field) |
| `social.promise_broken` | `a promise was broken (#{id})` (verified: payload carries only the promise id — no from/to) |
| `social.secret_seeded` | `a secret took root with {agent}` (fields verified at impl) |
| `social.chest_taken` | `{taker} raided {owner}'s chest at ({x},{y})` **(alert)** |
| `social.hailed` | `{from} hailed {to} (until t{until})` |
| `social.hail_met` | `{from} met {to}` |
| `social.hail_expired` | `{from}'s hail to {to} lapsed` |

### governance (meeting.* / norm.*)

| Type | Template |
|---|---|
| `meeting.convened` | `meeting convened` + place (verified: `MeetingPlacePayload` carries the place only, no agents list) |
| `meeting.opened` | `meeting opened` |
| `meeting.turn_taken` | `{agent} spoke at the meeting` |
| `meeting.proposal_tabled` | `{agent} proposed: "{text}"` |
| `meeting.proposal_resolved` | `proposal {outcome}: "{text}"` (+ tally when present) |
| `meeting.proposal_rephrased` | `norm rephrased: "{text}"` |
| `meeting.closed` | `meeting closed` |
| `meeting.place_designated` | `meeting place set at ({x},{y})` |
| `meeting.convention_established` | `meeting convention: {open time} at ({x},{y}) ({source})` |
| `norm.violated` | `{agent} violated a norm (#{norm_id})` **(alert)** (verified: payload carries `norm_id`, never the norm text) |

### gru / chronicle / metatron

| Type | Template |
|---|---|
| `gru.emerged` | `the gru emerged at ({x},{y})` |
| `gru.moved` | `the gru prowls to ({x},{y})` |
| `gru.sighted` | `{agent} sighted the gru` |
| `gru.attacked` | `the gru attacked {agent} · health → {health}` **(alert)** |
| `gru.withdrew` | `the gru withdrew` |
| `chronicle.entry` | `day {day}` (+ ` · {thread}`) `: {text, truncating}` |
| `metatron.charge_regenerated` | `a charge regenerated` |
| `metatron.nudged` | `Metatron {form} → {targets as names}: "{text}"` |

### cog (labeled)

| Type | Template |
|---|---|
| `cog.thought` | `job={job} class={class} agent={Agent} pts={points} pred={predicted_wall_ms}ms` |
| `cog.outcome` | `job={job} {outcome} agent={Agent} stale={staleness_ticks}t wall={actual_wall_ms}ms` (+ `kind={kind}` / `reason={reason}` when present) |
| `cog.recalibration_recommended` | `tier={tier} est={estimate_s_per_pt}s/pt spikes={spike_rate} window={window}` |

## 4. Color-role contract (extends chronicle-grammar.md §Color roles)

Roles, never raw colors: `dim` (tick/time, fallback payloads) · `family/<name>` (type column tint per family; `clock` keeps yellow) · `name` (resolved agent names, existing bold green) · `speech` (quoted utterances, brightest) · `emphasis` (amounts/kinds/causes/coords) · `alert` (whole-line for the four alert types) · `selection` (inspect-mode marker/row, existing reverse). Distinct assignments per family; exact palette fixed at implementation and recorded in the reconciled pattern doc.

## 5. Detail pane (inspect Mode 2)

```
┌─ CHRONICLE · paused — j/k select · J/K scroll detail ─┐
│   8801 08:09  agent.foraged   Ash foraged at (14,9)   │
│▌  8846 08:11  social.conversation_turn  Ash→Rowan "…" │
│   8850 08:12  agent.died      Birch died: exposure    │
├─ DETAIL · seq 1202 ───────────────────────────────────┤
│ {                                                     │
│   "seq": 1202, "tick": 8846,                          │
│   "type": "social.conversation_turn",                 │
│   "payload": { … "speaker": 1,   // Rowan … }         │
│ }                                                     │
│ … (+12 more — J to scroll)        [future: actions]   │
└───────────────────────────────────────────────────────┘
```

- Always visible while inspecting; shows `chronSelectionBase()`'s event via `formatInspector` (verbatim payload, `// name` annotations, bytes never rewritten).
- Pane rows = `min(rows/2, 14)`; list keeps the remainder (≥5). Content overflow scrolls with `J`/`K`; footer shows remaining-line count. Oversized payloads (`world.migrated`) are windowed, all lines reachable by scroll (FR-011).
- **Extension point (FR-009)**: the pane's bottom-right slot `[future: actions]` and the reserved `⏎` key are the documented attachment surface for jump-off actions. Code marks it with one hook (`detailActions(e store.Event) []detailAction`, returning nil today); panels/chronicle.md documents it.

## 6. Keymap delta (reconciled into patterns/keymap.md)

| Key | Before | After |
|---|---|---|
| `j`/`k`, `g`/`G` | move selection | unchanged (also resets detail scroll) |
| `⏎` | expand/collapse inline inspector | reserved no-op (future jump-off actions) |
| `J`/`K` | — | scroll detail pane |
| `r`/`a`/`t`, resume behavior | filters / tail-follow restore | unchanged |

## 7. Sweep guarantee (SC-001)

The sweep test fails when: a fixture type has no registry entry; a fixture type's digest falls back to raw JSON on its sample payload; a registry key is absent from the fixture; or a backticked concrete type in `docs/wiki/event-types.md` is absent from the fixture. Adding an event type to the system therefore forces either a digest entry or a deliberate, visible fixture change.
