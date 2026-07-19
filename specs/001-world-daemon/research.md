# Research: World Daemon & Time Substrate

**Phase 0 output** — every Technical Context unknown resolved as a decision.

## R1. SQLite driver: pure-Go vs cgo

- **Decision**: `modernc.org/sqlite` (pure-Go transpiled SQLite), WAL journal mode,
  `synchronous=NORMAL`.
- **Rationale**: no cgo keeps cross-compilation and CI trivial and the binary static;
  throughput ceiling (~thousands of inserts/sec batched per tick-commit) is orders of
  magnitude above v1 event volume. WAL lets an attached reader (status queries) coexist
  with the single writer goroutine.
- **Alternatives considered**: `mattn/go-sqlite3` (cgo — faster, but build friction on
  darwin/arm64 CI and cross-compiles); `bbolt`/flat JSONL (no SQL querying for the
  chronicle and Metatron digests later — the event log is exactly the thing future
  features will query, keep it SQL); Postgres (a server dependency for a homelab
  single-process game is unjustifiable).

## R2. Tick granularity and clock model

- **Decision**: **1 tick = 1 game second.** Speed is a multiplier of game-seconds per
  real-second: `1x` = real-time, **default `4x`** (1 game-min per 15 real-sec), `0` =
  paused (a flag, not a multiplier), `max` = uncapped (as-fast-as-affordable). The loop
  is a fixed-timestep scheduler: at speed `s`, one tick fires every `1/s` real seconds;
  at `max` it spins back-to-back with a small yield.
- **Rationale**: game-second granularity gives later features (movement, conversations,
  the gru's sight checks) room without clock redesign; the grounding session fixed the
  *compression ratio*, not the tick size. Integer ticks (`int64` ordinal) + derived game
  time avoid float drift over weeks-long runs.
- **Alternatives considered**: 1 tick = 1 game minute (too coarse for movement/combat
  later; forces a redesign exactly when TASK-5 lands); real-time delta stepping
  (non-deterministic — rejected outright by SC-006).

## R3. Determinism strategy

- **Decision**: single-goroutine simulation loop owning all state; `math/rand/v2`
  PCG source seeded from the world manifest; **all external inputs (player/time-control
  commands) are enqueued and applied only at tick boundaries, and every applied command
  is itself recorded as an event**. Event payloads serialize as canonical JSON (sorted
  keys, no floats where integers serve). Wall-clock time never enters sim state — it
  appears only in log metadata columns.
- **Rationale**: determinism = same seed + same tick-stamped input sequence → same
  history. Recording applied commands as events makes the log the complete input record,
  so replay needs nothing else (SC-006, and the byte-identical two-run test).
- **Alternatives considered**: multi-goroutine sim with locks (unprovable determinism);
  hashing state per tick for verification (kept as a cheap `state_hash` on snapshots
  only — full per-tick hashing is wasted work at v1 scale).

## R4. Persistence model: event sourcing with snapshot bounds

- **Decision**: the event log is the source of truth; world state is a reducer over
  events (`state.Apply(event)`), used identically live and during replay. Snapshots =
  serialized reducer state (JSON blob, zstd-free — plain for debuggability) every
  N ticks (default 3600 ticks = 1 game hour) plus on graceful shutdown and on pause.
  Recovery = load latest valid snapshot → replay events with `seq > snapshot.seq`.
  A corrupt snapshot falls back to the previous one (older snapshots retained, pruned
  to the last 24).
- **Rationale**: one code path (the reducer) guarantees replay equals live; snapshot
  cadence bounds SC-003 recovery (< 1 game hour of events ≈ 3,600 events, well under
  10 s). Append-only is enforced in-schema with triggers raising on UPDATE/DELETE.
- **Alternatives considered**: mutable state table + log as audit trail (two write
  paths that drift — the classic bug the spec's "log is source of truth" forbids);
  replay-from-genesis always (violates SC-003 for long runs).

## R5. Client protocol transport and framing

- **Decision**: Unix domain socket at `<savedir>/daemon.sock`; newline-delimited JSON
  (JSON-lines). Requests: `{"id":n,"cmd":"...","args":{...}}`; responses:
  `{"id":n,"ok":true,"data":{...}}` or `{"id":n,"ok":false,"error":"..."}`; server-push
  events after `subscribe`: `{"push":"event","event":{...}}`. Multiple concurrent
  clients allowed; all are equal (trusted single-operator host). Slow subscribers get a
  bounded buffer; overflow drops the *subscription* (client re-syncs from the log by
  `seq`), never blocks the sim loop.
- **Rationale**: UDS in the save dir needs no port management, scopes naturally
  per-world, and gives filesystem permissions for free. JSON-lines is trivially
  debuggable (`nc -U` works) and is what a Bubble Tea client (TASK-3) can consume with
  zero codegen. Cursor-based re-sync via `seq` makes drops harmless.
- **Alternatives considered**: gRPC (codegen + dependency weight for a localhost
  single-operator protocol — no); TCP localhost (port collisions across multiple
  worlds; UDS is strictly simpler); length-prefixed binary framing (debuggability loss
  for no needed throughput).

## R6. Daemon lifecycle: detach, stop, crash

- **Decision**: `scriptworld daemon <dir>` runs the loop in the foreground (the
  primitive). `scriptworld start <dir>` detaches by re-exec'ing itself (`daemon`
  subcommand) with stdio to `<savedir>/daemon.log`, writing `<savedir>/daemon.pid`.
  `stop` sends a `shutdown` command over the socket (graceful: final snapshot, clean
  close); SIGTERM does the same; SIGKILL is the crash path recovery is tested against.
  Stale pid/sock files (crash leftovers) are detected on start via pidfile liveness
  check and swept.
- **Rationale**: self-re-exec avoids launchd/systemd coupling while keeping "always-on
  on the homelab" a one-command affair; the foreground primitive is what e2e tests and
  a future launchd plist both use. Crash-then-resume is a first-class tested scenario,
  not an error path.
- **Alternatives considered**: require launchd from day one (platform-locks the e2e
  tests); double-fork classic daemonization (not portable in Go, re-exec is the
  idiomatic equivalent).

## R7. Overrun handling (auto-slow)

- **Decision**: the scheduler measures actual tick duration; when the loop cannot hold
  the requested rate (sustained over a 5 s window), it lowers the *effective* rate to
  what it can sustain, emits a `clock.degraded` event (surfacing FR-012), and keeps
  game-time bookkeeping honest — game time only advances by ticks actually executed.
  When headroom returns, effective rate climbs back to the requested speed.
- **Rationale**: the grounding session's resilience stance — degrade, never drop.
  Because game time is derived from tick count (R2), slowing the tick rate slows game
  time with zero state corruption by construction.
- **Alternatives considered**: skip ticks to catch up (breaks determinism and event
  continuity); block silently (violates "MUST surface that it is doing so").

## R8. World identity & manifest

- **Decision**: `world.json` manifest at save-dir root: `{name, seed, created_at,
  format_version, tick_game_seconds: 1}`. The save dir *is* the world identity; no
  global registry. `scriptworld new <dir> [--name] [--seed]` creates layout + schema;
  refuses non-empty dirs.
- **Rationale**: matches FR-009/"runs cleanly separable" — cp -r of the dir is a full
  archive. Explicit `format_version` buys future migration room now, when it's free.
- **Alternatives considered**: worlds registry in `~/.scriptworld` (global state the
  grounding explicitly rejected: "never global; runs cleanly separable").
