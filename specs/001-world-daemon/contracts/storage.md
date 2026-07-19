# Contract: save directory & storage schema

## Save directory layout (one world run)

```text
<world-dir>/
├── world.json     # manifest: {name, seed, created_at, format_version, tick_game_seconds}
├── world.db       # SQLite (WAL): events, snapshots, meta
├── world.db-wal   # SQLite WAL sidecars (present while open)
├── world.db-shm
├── daemon.sock    # UDS — exists only while a daemon is running
├── daemon.pid     # pid of the running daemon — swept if stale
├── daemon.log     # stdio of detached daemon (append)
└── agents/        # flat files owned by later features (persona.md, soul.md, …); created empty
```

`cp -R <world-dir>` of a *stopped* world is a complete, restorable archive (FR-009).
Nothing about a run lives outside its directory.

## SQLite schema (`world.db`, format_version 1)

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous  = NORMAL;

CREATE TABLE meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
) WITHOUT ROWID;
-- keys: format_version, seed, created_at

CREATE TABLE events (
  seq       INTEGER PRIMARY KEY,          -- assigned by store, contiguous from 1
  tick      INTEGER NOT NULL,
  type      TEXT    NOT NULL,
  payload   TEXT    NOT NULL,             -- canonical JSON (sorted keys)
  wall_time TEXT    NOT NULL              -- RFC3339; observability only
);
CREATE INDEX events_tick ON events(tick);
CREATE INDEX events_type ON events(type);

-- Append-only, enforced in-schema (spec FR-006):
CREATE TRIGGER events_no_update BEFORE UPDATE ON events
  BEGIN SELECT RAISE(ABORT, 'events is append-only'); END;
CREATE TRIGGER events_no_delete BEFORE DELETE ON events
  BEGIN SELECT RAISE(ABORT, 'events is append-only'); END;

CREATE TABLE snapshots (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  tick       INTEGER NOT NULL,
  seq        INTEGER NOT NULL,            -- last event seq folded into state
  state      TEXT    NOT NULL,            -- canonical JSON reducer state
  state_hash TEXT    NOT NULL,            -- sha256 hex of state bytes
  wall_time  TEXT    NOT NULL
);
```

## Write discipline

- One transaction per executed tick that produced events: append events + (on cadence)
  snapshot, commit. No event is acknowledged to subscribers before its transaction
  commits.
- `events` receives INSERTs only; triggers abort anything else.
- Snapshot cadence: every 3600 ticks + graceful shutdown + pause. Retention: newest 24
  (pruning deletes only from `snapshots`, never `events`).

## Recovery procedure (daemon start against existing dir)

1. Validate `world.json` ↔ `meta` (format_version, seed).
2. Pick newest snapshot whose `state_hash` verifies; on failure, try older; if none,
   start from genesis state.
3. Replay events `seq > snapshot.seq` through the reducer (same code path as live).
4. Resume loop with recovered clock state (tick, paused flag, speed) — a world killed
   while paused wakes paused (edge case in spec).
5. Log gap check: `seq` must be contiguous; a gap is a fatal integrity error (refuse to
   run, print recovery guidance) rather than silent corruption.
