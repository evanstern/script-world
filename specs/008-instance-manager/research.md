# Research: World Instance Manager

**Feature**: `008-instance-manager` | **Date**: 2026-07-21

No NEEDS CLARIFICATION markers remained after Technical Context was filled — the
codebase answers every technical question directly. This file records the design
decisions and the alternatives weighed.

## D1 — Discovery model: worlds-home scan + advisory registry (no process scanning)

**Decision**: `ps` discovers candidate worlds from two sources: (a) a directory scan of
the worlds home (every immediate subdirectory containing `world.json`), and (b) an
advisory registry file of known out-of-home worlds (`name → path`), refreshed whenever a
world is created (`new`) or a daemon boots (`daemon.Run`, which `start` spawns).
Liveness is then re-proven per candidate (D2) — the registry never asserts "running".

**Rationale**: Worlds are self-contained directories that can live anywhere (grounded
"one directory = one world, never global" decision — `docs/wiki/world-save-directory.md`).
There is no central runtime state today: the pidfile and socket live *inside* the world
dir (`internal/world/world.go` `PidPath()`/`SockPath()`). Machine-wide enumeration
therefore needs a candidate list; scan + advisory registry provides it without making
any world depend on external state (FR-008). Registering from `daemon.Run` (not from the
`start` client) means even foreground `scriptworld daemon <dir>` runs become visible.

**Alternatives considered**:
- *Scan the process table* (`ps aux | grep scriptworld`): platform-fragile, breaks on
  renamed binaries, cannot recover the world dir reliably from argv, and gives no way to
  list stopped worlds (`ps --all`). Rejected.
- *Central runtime dir of pidfiles* (`~/.scriptworld/run/<name>.pid`): creates a second
  authoritative copy of per-world lifecycle state; a crashed daemon would leave the two
  copies disagreeing. The world dir stays the sole source of truth; the registry is a
  pointer cache only. Rejected.
- *Registry as the only source (no home scan)*: a freshly copied-in world under the
  worlds home would be invisible until first started. Scan makes the home
  self-describing. Rejected.

## D2 — Liveness: pidfile pre-filter, then a bounded status round-trip; parallel probes

**Decision**: A world is **running** iff its daemon answers a `status` IPC call within a
short per-world budget (dial + call, ~1s). The existing pidfile check
(`daemon.IsRunning`: read `daemon.pid`, `kill(pid, 0)`) runs first as a cheap pre-filter
— no live pid means no dial. A live pid whose daemon does not answer in time is shown as
`unresponsive` (never `running`); a dead pid with leftover files is simply not running
(the daemon already sweeps stale pidfiles on next boot, `acquirePidfile`). All candidate
probes run concurrently so one wedged daemon cannot stall the listing (SC-001: < 2s
machine-wide).

**Rationale**: FR-002 demands live evidence only. The daemon's `status` reply
(`ipc.StatusData`) already carries everything `ps` must display — tick, game time,
paused, speed, pid, and `LLM != nil` for inference-enabled (FR-013) — so responsiveness
doubles as the data fetch. `ipc.Dial` already takes a 2s dial timeout; the probe wraps
dial+call in a goroutine with its own deadline rather than adding protocol surface.

**Alternatives considered**:
- *pidfile-only liveness*: shows `running` for a wedged/deadlocked daemon and can't
  report tick/speed/LLM without dialing anyway. Rejected (FR-001/FR-002).
- *New daemon-side broadcast/heartbeat protocol*: unnecessary — spec assumption already
  states `ps` is client-side aggregation over existing evidence. Rejected.

## D3 — Name-vs-path rule and resolution order

**Decision**: An argument is a **path** iff it contains a path separator (`/`) or begins
with `.` or `~`; otherwise it is a **name**. (This subsumes `..` and `./name`.) A name
resolves: (1) worlds-home entry `<home>/<name>`; (2) registry entry `name`. If both
exist and point at different directories, the command refuses as ambiguous and prints
both paths (FR-011). An unresolvable name exits 1 naming the worlds home searched and
suggesting `scriptworld ps --all` (FR-007). Resolution happens in one shared helper used
by every per-world command; path-shaped arguments bypass it entirely and hit today's
code path unchanged (FR-006, FR-012, SC-003).

**Rationale**: Matches the spec's Assumptions verbatim; historical invocations all used
`.`-, `~`-, or separator-containing forms, so the bare-name namespace is free to claim.

**Alternatives considered**:
- *"Exists as a directory in CWD" heuristic*: non-deterministic — the same command would
  mean different things in different CWDs. Spec explicitly requires deterministic
  resolution; `./name` remains the escape hatch. Rejected.

## D4 — Worlds home location and override

**Decision**: The scriptworld home is `~/.scriptworld`, overridable with the
`SCRIPTWORLD_HOME` environment variable. Derived paths: worlds home
`$SCRIPTWORLD_HOME/worlds`, registry `$SCRIPTWORLD_HOME/known_worlds.json`. All
discovery, creation, and name resolution read the same helper, so the override is
honored everywhere consistently (spec edge case).

**Rationale**: One env var moves the whole management surface (worlds + registry)
together — splitting them apart (separate overrides) invites a registry that points into
a home that isn't scanned. Follows the ollama/docker single-home convention the spec
invokes.

**Alternatives considered**:
- *`SCRIPTWORLD_WORLDS_HOME` (worlds dir only)*: leaves the registry location ambiguous.
  Rejected in favor of the single root.
- *Config file for the override*: no config file exists today and the spec only demands
  environment/config; env-only is the smallest honest surface. A config file can layer
  on later without breaking the env var.

## D5 — `new` argument semantics

**Decision**: `scriptworld new <name>` (bare word) creates `<worlds-home>/<name>` with
manifest name `<name>`, creating the worlds home on first use; it refuses (exit 1) if
the target directory already exists (FR-004). `scriptworld new <path-shaped-arg>` keeps
today's exact behavior: create at that path, manifest name from `--name` or the basename
(FR-012 "keep the old create-at-path behavior"). The location override for named
creation is `--at <dir>`: `new aria --at /tmp/somewhere` creates `/tmp/somewhere` (the
path itself, not `<dir>/aria` — explicit is explicit) with manifest name `aria`, and
registers it in the known-worlds registry. Name validation (FR-009): non-empty, no path
separators, not flag-like (no leading `-`), usable as a directory name; enforced for the
bare-name form and for `--name`.

**Rationale**: Preserves both historical forms while making the bare word mean a name —
exactly the accepted breakage-with-guardrail in the spec's Assumptions. `--at` gives
custom-location users the same name-first ergonomics (US3) instead of forcing them back
to path-addressing.

**Alternatives considered**:
- *Reject path-shaped args to `new` with a redirect message*: spec allows either; keeping
  the old behavior is strictly more backward compatible and costs nothing. Rejected.
- *`--at <parent>` meaning `<parent>/<name>`*: surprising for users who typed a full
  target path; docker/ollama precedents don't compose paths this way. Rejected.

## D6 — Registry self-healing semantics

**Decision**: The registry (`known_worlds.json`) is a flat `name → {path}` map, written
atomically (temp file + rename). It is refreshed on `new` (custom-path) and on every
daemon boot (upsert by manifest name; also repairs a moved path). Entries whose
directory no longer contains a readable `world.json` are: skipped by bare-name
resolution (with the "missing" fact included if that name was asked for), shown as
`missing` in `ps --all`, and pruned on the next registry write. A corrupt or absent
registry file is treated as empty — never an error (FR-008: advisory, never required).
Worlds inside the worlds home are never written to the registry (the scan owns them);
a registry entry whose path resolves inside the current worlds home is ignored/pruned.

**Rationale**: Every read path tolerates lies; every write path heals them. A world
remains fully runnable and copyable with zero registry state (SC-004) because nothing
reads the registry except discovery/resolution conveniences.

**Alternatives considered**:
- *Prune aggressively at read time (rewrite on every `ps`)*: `ps` becoming a writer makes
  a read-only listing mutate global state and races concurrent daemon boots. Writes stay
  on write paths. Rejected.

## D7 — `ps` output shape

**Decision**: Human mode prints a column table: `NAME  STATE  PID  TICK  GAME TIME
SPEED  LLM  PATH` — state ∈ `running | paused | unresponsive | stopped | missing |
unreadable` (the latter four only under `--all`), LLM ∈ `on | off` (from `StatusData.LLM
!= nil` when running; from `llm.json` presence when stopped). No worlds → `no worlds
running` (exit 0). `--json` emits a JSON array of per-world objects reusing the existing
`status --json` vocabulary (`world`, `clock`, `daemon`, `llm`) plus `name`, `path`, and
`state`, consistent with FR-014; stopped worlds under `--all --json` carry their
last-known `clock` from the store exactly as offline `status` does today.

**Rationale**: Reuses the wire vocabulary clients already parse; the offline
last-known-state read (`store.LatestValidSnapshot` + `LastEventTick`) already exists in
`cmdStatus` and is extracted into a shared helper rather than duplicated.

**Alternatives considered**:
- *JSON object keyed by name*: an array preserves display order and survives duplicate
  names during ambiguity states. Rejected.
