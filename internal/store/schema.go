package store

// DDL is the format_version 1 schema from specs/001-world-daemon/contracts/storage.md.
// The events table is append-only, enforced in-schema by triggers.
const ddl = `
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS events (
  seq       INTEGER PRIMARY KEY,
  tick      INTEGER NOT NULL,
  type      TEXT    NOT NULL,
  payload   TEXT    NOT NULL,
  wall_time TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS events_tick ON events(tick);
CREATE INDEX IF NOT EXISTS events_type ON events(type);

CREATE TRIGGER IF NOT EXISTS events_no_update BEFORE UPDATE ON events
  BEGIN SELECT RAISE(ABORT, 'events is append-only'); END;
CREATE TRIGGER IF NOT EXISTS events_no_delete BEFORE DELETE ON events
  BEGIN SELECT RAISE(ABORT, 'events is append-only'); END;

CREATE TABLE IF NOT EXISTS snapshots (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  tick       INTEGER NOT NULL,
  seq        INTEGER NOT NULL,
  state      TEXT    NOT NULL,
  state_hash TEXT    NOT NULL,
  wall_time  TEXT    NOT NULL
);
`
