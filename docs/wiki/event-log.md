---
name: event-log
description: Append-only SQLite events table — the source of truth for a world run; contiguous seq, in-schema immutability triggers, WAL
kind: component
sources:
  - internal/store/store.go
  - internal/store/schema.go
verified_against: 8be4440aae8d108884080cb6476782d2f11ad165
---

# Event log

The `events` table in `world.db` is a world run's authoritative history: every
simulation event and every applied command lands here, in order, and can never be
modified. World state is derived from it (event sourcing); snapshots merely accelerate
that derivation.

## How it works

`internal/store.Store` opens SQLite via the pure-Go `modernc.org/sqlite` driver with
`journal_mode=WAL`, `synchronous=NORMAL`, and `SetMaxOpenConns(1)` — the sim loop is
the single writer, and one connection sidesteps SQLITE_BUSY entirely.

Schema (in `schema.go`): `events(seq INTEGER PRIMARY KEY, tick, type, payload,
wall_time)` with indexes on `tick` and `type`. Two triggers, `events_no_update` and
`events_no_delete`, `RAISE(ABORT, 'events is append-only')` — immutability is enforced
in-schema, not by convention.

- `AppendEvents(events)` assigns contiguous seqs (`lastSeq+1…`) and writes the batch in
  one transaction — one batch per tick, so no event is visible to subscribers before
  its tick commits. `lastSeq` is an `atomic.Int64` because the loop writes it while IPC
  sessions read it.
- `ReplayEvents(sinceSeq, fn)` / `EventsSince(sinceSeq, limit)` stream history in seq
  order — used by recovery, subscribe-replay, and `tail`.
- `CheckContiguity()` verifies seq runs exactly 1..N; a gap is a fatal integrity error
  and [[daemon-lifecycle]] refuses to run on a holed log.
- `Event.Payload` is canonical JSON (struct-marshaled, fixed field order) so histories
  are byte-comparable; `wall_time` is observability metadata, excluded from determinism
  comparisons.

## Connections

[[sim-loop]] is the only writer; [[sim-state-reducer]] consumes events in replay;
[[snapshots]] bound how much of the log recovery must re-read; [[ipc-server]] reads it
for subscribe-replay and gap-fill; [[event-types]] catalogs what lands in it.

## Operational notes

Event volume at v1 scale is trivial for SQLite (<1M rows per 30-day run). The meta
table (same file) stores `seed`/`format_version` for cross-checking against
`world.json`. Future features (chronicle narration, Metatron digests) are expected to
query this table by `tick`/`type` — the indexes exist for them.
