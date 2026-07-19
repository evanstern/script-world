# Data Model: World Daemon & Time Substrate

**Phase 1 output.** Entities from the spec mapped to concrete shapes. DDL lives in
[contracts/storage.md](contracts/storage.md); wire shapes in
[contracts/client-protocol.md](contracts/client-protocol.md).

## World (run)

One save directory = one world run = one daemon process at a time.

| Field | Type | Notes |
|---|---|---|
| `name` | string | display name; from `world.json` |
| `seed` | uint64 | RNG seed; immutable after `new` |
| `created_at` | RFC3339 | wall time of creation (metadata only, never sim input) |
| `format_version` | int | save-layout version, starts at 1 |
| `tick_game_seconds` | int | fixed at 1 in v1; recorded so it can never silently change |

Validation: save dir must contain `world.json` + `world.db` with matching
`format_version`; daemon refuses to start otherwise. Exactly one daemon per dir,
enforced by pidfile liveness + socket bind.

## Tick

Not stored as rows — an ordinal coordinate stamped on everything else.

| Field | Type | Notes |
|---|---|---|
| `tick` | int64 | 0-based count of executed ticks since world creation |
| game time | derived | `game_epoch + tick * 1s`; game epoch is day 1, 06:00 |

Invariant: `tick` only increments, by exactly 1, only while unpaused. Game time is
never stored independently of `tick` (no drift possible).

## Event

Append-only history; the source of truth. Also the complete input record: applied
commands are events too (R3).

| Field | Type | Notes |
|---|---|---|
| `seq` | int64 PK | global order, contiguous, assigned by the store |
| `tick` | int64 | tick the event occurred in |
| `type` | string | namespaced: `world.created`, `clock.paused`, `clock.resumed`, `clock.speed_set`, `clock.degraded`, `clock.recovered`, `sim.day_started`, `sim.night_started`, `agent.moved`, `agent.slept`, `daemon.started`, `daemon.stopped` |
| `payload` | canonical JSON | sorted keys; deterministic bytes (SC-006) |
| `wall_time` | RFC3339 | observability only — excluded from determinism comparisons |

Invariants: INSERT-only (schema triggers reject UPDATE/DELETE); `seq` strictly
increasing; events for tick T are contiguous; one transaction per tick batch.

## Snapshot

Recovery accelerator, never authority (log wins on any conflict).

| Field | Type | Notes |
|---|---|---|
| `id` | int64 PK | |
| `tick` | int64 | tick the snapshot captures (after applying all its events) |
| `seq` | int64 | last event seq folded into this snapshot |
| `state` | JSON blob | serialized reducer state (clock state + sim state) |
| `state_hash` | string | SHA-256 of canonical state bytes; validates integrity + determinism checks |
| `wall_time` | RFC3339 | metadata |

Cadence: every 3600 ticks (1 game hour) + on graceful shutdown + on pause. Retention:
last 24. Recovery: newest snapshot whose `state_hash` verifies → replay `seq >
snapshot.seq`; on verify failure fall back to the previous snapshot.

## Clock state (part of reducer state)

| Field | Type | Notes |
|---|---|---|
| `tick` | int64 | current tick |
| `paused` | bool | pause is a flag, not a speed |
| `speed` | string | `"1x"`, `"4x"` (default), `"8x"`, …, `"max"` — requested rate |
| `effective_rate` | float | ticks/sec actually sustained; = requested unless degraded |
| `degraded` | bool | true while auto-slow is active (FR-012) |

Transitions (each records an event):
`running --pause--> paused --resume--> running`;
`speed_set` allowed in either state (takes effect on resume if paused);
`degraded` toggles only while running, driven by the scheduler's 5 s window (R7).
Durable across restart: pause state, speed, tick all come back exactly (FR-008) —
they live in reducer state, hence in snapshots + replay.

## Client session

Ephemeral; never persisted; lifecycle fully decoupled from the sim.

| Field | Type | Notes |
|---|---|---|
| `conn` | UDS connection | JSON-lines framed |
| `subscribed` | bool | receiving event pushes |
| `cursor` | int64 | last `seq` delivered; client re-syncs from it after drops |

Invariant: a session's death (EOF, error, buffer overflow) closes that session only.
The sim loop never blocks on any session (bounded per-session push buffer).

## Placeholder sim state (v1 scaffolding, replaced by TASK-5+)

Two wanderer entities on an abstract 16×16 grid + a day/night flag. Each game-minute
boundary each wanderer moves (seeded RNG) emitting `agent.moved`; at 22:00/06:00
boundaries `sim.night_started`/`sim.day_started` fire and wanderers `agent.slept`.
Exists solely to push deterministic events through log/snapshot/protocol paths.
