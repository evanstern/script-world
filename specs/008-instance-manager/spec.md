# Feature Specification: World Instance Manager

**Feature Branch**: `008-instance-manager`

**Created**: 2026-07-21

**Status**: Draft

**Input**: User description: "Instance manager for running worlds ('promptworld ps'-style lifecycle management, docker/ollama ergonomics). Problem: users run multiple world daemons at once, lose track of them, and concurrent daemons clobber the shared local LLM host and step on each other. Proposal: (1) name-based world addressing with a default worlds home at ~/.promptworld/worlds/<name> — `promptworld new <worldname>` creates there by default; a --world=/path/to flag (or explicit path) still supports arbitrary locations, preserving the grounded 'one directory = one world, never global, cleanly separable' decision. (2) Manager subcommands: `promptworld ps` lists running (and optionally all known) worlds; `stop <name>`, `start <name>`, `status <name>`, and existing per-world commands accept a world name resolved against the worlds home, in addition to paths. Existing path-based invocations must keep working."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See everything that is running (Priority: P1)

A player who has been experimenting with several worlds types one command, from any
directory, and sees every world daemon currently alive on the machine: its name, whether
it is running or paused, its process id, current tick and in-game time, speed, and
whether inference (LLM use) is enabled for it. Dead leftovers (crashed daemons, stale
records) never appear as running.

**Why this priority**: This is the core pain — "we're stepping on our toes and don't know
what's running." Visibility alone lets the player find and stop the daemon that is
clobbering the shared LLM host, even with no other part of this feature built.

**Independent Test**: Start two worlds by path (existing commands), run `promptworld ps`
from an unrelated directory, and confirm both appear with live data; kill one with
SIGKILL and confirm it no longer shows as running.

**Acceptance Scenarios**:

1. **Given** two world daemons running (started from any locations), **When** the user
   runs `promptworld ps` from any working directory, **Then** both worlds are listed
   with name, state (running/paused), pid, tick, in-game time, speed, and LLM
   enabled/disabled — and the command exits 0.
2. **Given** no daemons running, **When** the user runs `promptworld ps`, **Then** an
   empty listing (header only or "no worlds running") is printed and the command exits 0.
3. **Given** a daemon that was SIGKILLed (stale pidfile/socket left behind), **When**
   the user runs `promptworld ps`, **Then** that world is not shown as running.
4. **Given** stopped worlds exist in the worlds home or were previously started,
   **When** the user runs `promptworld ps --all`, **Then** stopped worlds are listed too,
   clearly marked as not running, with their last-known tick/game time.

---

### User Story 2 - Create and address worlds by name (Priority: P2)

A player creates a world with just a name — `promptworld new aria` — and it lands in the
default worlds home (`~/.promptworld/worlds/aria`). From then on, every command that
previously took a directory accepts the name: `promptworld start aria`,
`promptworld ui aria`, `promptworld stop aria`. Players who want full control of
location keep it: an explicit path (or a location flag on `new`) works exactly as today,
and the resulting world is still a self-contained, copyable directory.

**Why this priority**: Names are what make the manager ergonomic (docker/ollama feel),
but visibility (P1) is useful without them; they are the second slice.

**Independent Test**: Run `promptworld new testworld`, verify the directory appears
under the worlds home with a complete world in it, then run
`start`/`status`/`stop testworld` from another directory and confirm they resolve to it.

**Acceptance Scenarios**:

1. **Given** the worlds home does not yet exist, **When** the user runs
   `promptworld new aria`, **Then** the worlds home is created, a complete world is
   created at `<worlds-home>/aria`, and the world's recorded name is `aria`.
2. **Given** a world `aria` already exists in the worlds home, **When** the user runs
   `promptworld new aria`, **Then** the command refuses with a clear error and exit
   code 1, leaving the existing world untouched.
3. **Given** a world created by name, **When** the user runs any per-world command
   (`start`, `stop`, `status`, `ui`, `attach`, `tail`, `pause`, `resume`, `speed`,
   `metatron`, `llm`, `calibrate`, `daemon`) with that name from any directory,
   **Then** the command operates on that world.
4. **Given** the user passes an explicit path (contains a path separator, or `.`/`..`/
   `~` forms), **When** any per-world command runs, **Then** it is treated as a
   directory path exactly as before this feature (backward compatible).
5. **Given** the user runs `promptworld new` with a location override for a custom
   path, **When** creation succeeds, **Then** the world lives at that path, works with
   all path-based commands, and remains a self-contained copyable directory.
6. **Given** a name that does not resolve to any known world, **When** a per-world
   command runs, **Then** it fails with exit 1 and an error that names the worlds home
   it searched and suggests `promptworld ps --all`.

---

### User Story 3 - Manage worlds outside the worlds home by name (Priority: P3)

A player who keeps a world at a custom location starts it once by path; from then on the
manager knows about it — it appears in `ps`/`ps --all` and can be stopped, inspected,
and re-started by its name, without the player remembering the path.

**Why this priority**: Convenience layer over P1+P2 for the escape-hatch users; the
feature is viable without it (path users can keep using paths).

**Independent Test**: Create a world at a custom path, start it by path, confirm
`promptworld ps` shows it by name and `promptworld stop <name>` stops it; confirm the
record survives to `ps --all` after stopping and that deleting the directory makes the
manager forget it gracefully.

**Acceptance Scenarios**:

1. **Given** a world at a custom path started by path, **When** the user runs
   `promptworld ps`, **Then** the world appears under its recorded name with its data.
2. **Given** that world running, **When** the user runs `promptworld stop <name>`,
   **Then** the daemon stops cleanly, same as stopping by path.
3. **Given** a remembered custom-path world whose directory has been deleted or moved,
   **When** the user runs `ps --all` or addresses it by name, **Then** the manager
   reports it as missing (or silently forgets it in listings) rather than erroring
   confusingly, and never shows it as running.
4. **Given** a name collision (a remembered custom-path world and a worlds-home world
   share a name), **When** the user addresses that name, **Then** the command refuses
   as ambiguous, listing both candidates with their paths so the user can use a path.

### Edge Cases

- A bare name argument that also matches a directory of the same name in the current
  working directory: resolution is deterministic (see Assumptions) — worlds-home name
  wins; the user can force the directory with `./name`.
- `ps` while a daemon is mid-shutdown or mid-startup: the world is shown with live data
  if its daemon answers, otherwise as not running — never a hang; per-world probes have
  a short timeout so one wedged daemon cannot stall the listing.
- Two daemons started near-simultaneously for the same world: unchanged — the existing
  one-daemon-per-world guarantee (pidfile) holds; the manager reports the survivor.
- A world in the worlds home whose directory is corrupt (unreadable manifest): `ps
  --all` lists it as unreadable rather than aborting the whole listing.
- Names that look like flags (leading `-`), contain path separators, or are empty:
  rejected at `new` time with a clear message; name rules are documented.
- The worlds home path is user-overridable (environment/config); all name resolution
  and discovery follow the override consistently.
- `stop` remains idempotent by name exactly as it is by path ("not running" exits 0).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide a `ps` command that lists all currently running
  world daemons machine-wide, regardless of where their directories live or which
  directory the command is run from, showing at minimum: name, run state
  (running/paused), pid, current tick, in-game time, speed, and whether inference is
  enabled.
- **FR-002**: `ps` MUST determine "running" only from live evidence (process liveness /
  daemon responsiveness), never from recorded state alone; stale records and leftover
  files from crashes MUST NOT produce false "running" entries.
- **FR-003**: `ps --all` MUST additionally list known stopped worlds (everything in the
  worlds home plus previously seen custom-path worlds) with last-known tick and game
  time, clearly distinguished from running ones.
- **FR-004**: `new <worldname>` MUST create a world named `<worldname>` in the default
  worlds home (`~/.promptworld/worlds/<worldname>`), creating the worlds home on first
  use; it MUST refuse an already-used name without touching the existing world.
- **FR-005**: `new` MUST accept an explicit location override so worlds can be created
  at arbitrary paths, producing exactly the same self-contained world directory as
  today; created worlds MUST remain fully copyable/archivable directories with no
  external state required for the world to run.
- **FR-006**: Every per-world command (`daemon`, `start`, `stop`, `status`, `pause`,
  `resume`, `speed`, `ui`, `attach`, `tail`, `metatron`, `llm`, `calibrate`) MUST accept
  either a world name or a directory path; arguments containing a path separator or
  starting with `.`, `..`, or `~` MUST be treated as paths with today's exact behavior.
- **FR-007**: Name resolution MUST be deterministic and documented: a bare name resolves
  against the worlds home first, then against remembered custom-path worlds; an
  unresolvable name fails with exit 1 and a message naming the worlds home searched.
- **FR-008**: Any state the manager keeps about worlds outside the worlds home MUST be
  advisory and self-healing: it is refreshed when worlds are created/started, pruned or
  flagged when directories disappear, and never required for a world to run — the world
  directory remains the sole identity and source of truth (preserving the grounded
  "one directory = one world, never global, cleanly separable" decision).
- **FR-009**: World names MUST be validated at creation: usable as a directory name,
  no path separators, not flag-like, non-empty; violations fail with a clear message.
- **FR-010**: `stop <name>` MUST behave identically to today's path-based stop,
  including idempotency (stopping a non-running world exits 0) and the graceful-then-
  forceful escalation already specified.
- **FR-011**: Ambiguous names (one name matching multiple known worlds) MUST be refused
  with a listing of the candidates and their paths.
- **FR-012**: All pre-existing path-based invocations of every command MUST continue to
  work unchanged (backward compatibility), except `new`'s first positional argument,
  which becomes a name; path-shaped arguments to `new` (containing a separator) MUST
  either keep the old create-at-path behavior or fail with a message pointing at the
  location override — silently creating a world named like a path is not acceptable.
- **FR-013**: `ps` MUST make LLM contention visible at minimum by showing, per running
  world, whether inference is enabled; richer live-activity signals are desirable but
  optional in this feature.
- **FR-014**: `ps` output MUST support a machine-readable mode (consistent with the
  existing `status --json` convention) so scripts can enumerate running worlds.

### Key Entities

- **World**: A self-contained save directory (unchanged); its recorded name becomes its
  primary user-facing handle.
- **Worlds home**: The default parent directory (`~/.promptworld/worlds`) where
  name-created worlds live; overridable; scanned for discovery.
- **Known-worlds record**: Advisory memory of worlds living outside the worlds home
  (name → path), refreshed on create/start, self-healing, never authoritative for
  liveness or world content.
- **Running instance**: A world with a live daemon — always re-proven from process/
  daemon evidence at query time, never from records.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: From any directory, a user can enumerate every running world in one
  command in under 2 seconds, with zero false "running" entries across crash/stale
  scenarios.
- **SC-002**: A new user can create, start, observe, and stop a world knowing only a
  name — zero directory paths typed across the whole lifecycle.
- **SC-003**: 100% of pre-existing documented path-based command invocations behave
  unchanged (covered by the existing regression/e2e suite passing plus new
  name-resolution tests).
- **SC-004**: A stopped world remains a complete archive: copying its directory to a
  new machine or path and starting it there works with no manager state present.
- **SC-005**: When multiple worlds run at once, a user can identify which running
  worlds have inference enabled from a single `ps` invocation (measured: the
  motivating "who is clobbering the LLM?" question is answerable without attaching to
  any world).

## Assumptions

- **Name-vs-path rule**: an argument is a path iff it contains a path separator or
  begins with `.`, `..`, or `~`; otherwise it is a name. A bare name resolves to the
  worlds home first, then to remembered custom-path worlds; a same-named directory in
  the CWD is reachable as `./name`. This keeps every historical invocation (which in
  practice used `.`, relative, or absolute paths) working.
- **`new` argument change is accepted breakage-with-guardrail**: `new <bare-word>` now
  means a name in the worlds home. Since a bare word to old `new` meant a relative
  directory, path-shaped arguments are kept working (or clearly redirected), and the
  release notes call the change out.
- **Worlds home default** is `~/.promptworld/worlds`, overridable via an environment
  variable and/or flag; `ps` discovery and name resolution honor the override.
- **World name = directory name = manifest name** for name-created worlds; the existing
  `--name` flag on `new` becomes unnecessary for the default flow (name is the
  argument) but manifest name stays the display name for custom-path worlds.
- **Liveness probing** reuses the existing per-world evidence (pidfile liveness check,
  daemon status round trip) — no new daemon-side protocol is required beyond what
  `status` already provides; `ps` is a client-side aggregation.
- **LLM arbitration is out of scope**: serializing or rate-arbitrating concurrent
  daemons' access to a shared LLM host is follow-up work; this feature only makes the
  contention visible (FR-013).
- **`rm`/`logs` subcommands are out of scope** for this feature to keep it one
  deliverable; deletion stays `stop` + remove the directory (per the archiving story),
  and logs stay at `<world>/daemon.log`.
- **Single-user, single-machine scope**: the manager covers daemons on the local
  machine for the invoking user; cross-machine or multi-user coordination is out of
  scope.
