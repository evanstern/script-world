# Feature Specification: World Daemon & Time Substrate

**Feature Branch**: `001-world-daemon`

**Created**: 2026-07-18

**Status**: Draft

**Input**: User description: "World daemon & time substrate: the always-on Go daemon that hosts the promptworld simulation. Deterministic tick loop; game clock at default 1 game-min = 15 real-sec (4x compression) with speed range from real-time up to as-fast-as-affordable; pause as a first-class player verb; SQLite append-only event log plus periodic snapshots; per-world save directory with per-run flat files so runs are cleanly separable; client attach/detach protocol so a TUI client can connect and disconnect without stopping the world; world resumes from snapshot+log after daemon restart. Grounding: docs/design/grounded-assumptions.md (Time & posture, Stack, Cost & inference resilience). This is spec candidate #1 from the TASK-1 grounding session, linked to Backlog TASK-2."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The world runs without me (Priority: P1)

The player starts a world and walks away. The simulation keeps advancing on its own —
through the night, through the work day, while no viewer is connected. When the player
attaches a client hours later, the world has visibly moved on: game time has advanced at
the configured compression, and everything that happened while they were away is on the
record.

**Why this priority**: "Always-on ambient world" is the single most load-bearing revision
from the grounding session — promptworld is a persistent world, not a session game.
Every other feature (souls, chronicle, Metatron) assumes a substrate that never stops.

**Independent Test**: Start a world with no client attached; wait a known real-time
interval; attach and confirm game time advanced by the expected amount and events were
recorded for the whole interval.

**Acceptance Scenarios**:

1. **Given** a newly created world, **When** the daemon runs for 15 real minutes with no
   client attached, **Then** roughly 60 game minutes have elapsed (at default speed) and
   the event record covers the full interval with no gaps.
2. **Given** a running world with a client attached, **When** the client disconnects,
   **Then** the simulation continues advancing (verified by the event record) and a later
   re-attach shows the elapsed game time.
3. **Given** a running world, **When** a client attaches, **Then** it sees current world
   status (game time, speed, pause state) without interrupting the simulation.

---

### User Story 2 - Time is a dial in my hand (Priority: P2)

The player can pause the world outright — pause is an explicit verb, not a side effect of
looking away — resume it, and change its speed anywhere from real-time (1 game-min = 1
real-min) up to as fast as the host can sustain. The default is 4x compression: 1 game
minute per 15 real seconds.

**Why this priority**: Pause was named a required player verb in the grounding session,
and adjustable speed is what makes an ambient world livable (slow down for drama, speed
up through quiet nights). It is the first player-facing control surface of the game.

**Independent Test**: Issue pause/resume/speed commands over the client protocol and
observe game-clock behavior.

**Acceptance Scenarios**:

1. **Given** a running world, **When** the player issues pause, **Then** game time stops
   advancing and the world reports itself paused; real time passing changes nothing.
2. **Given** a paused world, **When** the player issues resume, **Then** game time
   continues from exactly where it stopped.
3. **Given** a running world at default speed, **When** the player sets speed to
   real-time, **Then** one game minute takes one real minute; **When** set back to
   default, **Then** one game minute takes 15 real seconds.
4. **Given** a client that detaches without pausing, **When** it reattaches, **Then** the
   world kept running the whole time (detach is not pause).

---

### User Story 3 - Nothing is ever lost (Priority: P3)

The daemon can be killed — crash, host reboot, deliberate stop — and the world picks up
where it left off. Every simulation event lands in an append-only record; periodic
snapshots keep recovery fast. Each world lives in its own save directory, so runs are
cleanly separable: a finished run can be archived or copied wholesale.

**Why this priority**: A weeks-long ambient run is only trustworthy if restarts are
lossless. This is the durability contract every later system (souls, chronicle, rumors)
builds on — but it only matters once Stories 1–2 give the world something to persist.

**Independent Test**: Kill the daemon mid-run, restart it against the same save
directory, and verify state and history are intact.

**Acceptance Scenarios**:

1. **Given** a running world, **When** the daemon process is killed and restarted against
   the same save directory, **Then** the world resumes with its game clock, pause state,
   speed, and full event history intact — no recorded event is lost.
2. **Given** two worlds created separately, **When** each runs, **Then** each writes only
   inside its own save directory, and copying one directory elsewhere captures that
   entire run.
3. **Given** a long-running world, **When** recovery happens, **Then** it completes from
   the latest snapshot plus the event record — without replaying the entire run from the
   beginning.

---

### Edge Cases

- **Tick overrun**: if advancing the simulation takes longer than the real-time budget
  for the current speed, the world slows down (auto-slow) rather than skipping ticks or
  dropping events — graceful degradation per the grounding session's resilience stance.
- **Client vanishes mid-session**: a client that disconnects abruptly (killed terminal,
  network hiccup) must not stall or crash the daemon; its session is simply cleaned up.
- **Multiple clients**: a second client attaching to the same world must either work
  (read-only observation) or be cleanly refused — never corrupt the session of the first.
- **Corrupt or missing snapshot**: recovery falls back to an older snapshot plus longer
  log replay; the event log is the source of truth.
- **Restart while paused**: a world stopped while paused comes back paused.
- **Command to a stopped world**: control commands sent when no daemon is running fail
  fast with a clear error, not a hang.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST run the simulation continuously as a background process,
  independent of any connected client, until explicitly stopped.
- **FR-002**: The system MUST advance the world in discrete, ordered ticks, and be
  deterministic: the same starting state and the same inputs MUST reproduce the same
  sequence of world states and events.
- **FR-003**: The system MUST maintain a game clock mapping real time to game time, with
  a default compression of 1 game minute per 15 real seconds (4x).
- **FR-004**: The system MUST support changing the speed at runtime, over the range from
  real-time (1:1) up to as fast as the host can sustain, taking effect promptly.
- **FR-005**: The system MUST treat pause as a first-class command: pause halts game time
  until an explicit resume; client disconnection MUST NOT pause the world.
- **FR-006**: The system MUST record every simulation event in an append-only event log,
  ordered and attributed to the tick in which it occurred.
- **FR-007**: The system MUST take periodic snapshots of world state so that recovery
  replays only the events since the latest usable snapshot.
- **FR-008**: After a daemon restart against an existing save directory, the system MUST
  resume the world from snapshot plus event log with no loss of recorded events, and
  MUST preserve game time, speed, and pause state.
- **FR-009**: The system MUST keep everything belonging to one world run inside one save
  directory — including the run's flat files (agent personas/souls, charter, and similar
  documents owned by later features) — so runs are cleanly separable and archivable.
- **FR-010**: The system MUST expose a local client protocol through which a client can
  attach, query world status (game time, tick, speed, pause state, uptime), subscribe to
  events as they happen, issue time-control commands (pause, resume, set-speed), and
  detach — all without interrupting the simulation.
- **FR-011**: The system MUST tolerate abrupt client disconnection and repeated
  attach/detach cycles without degradation.
- **FR-012**: When the simulation cannot keep up with the requested speed, the system
  MUST degrade gracefully by slowing effective speed — never by dropping events or
  corrupting state — and MUST surface that it is doing so.
- **FR-013**: The system MUST provide commands to create a new world, start/stop the
  daemon for a world, and report daemon status from the command line.

### Key Entities

- **World (run)**: one simulation instance bound to one save directory; owns its clock,
  event history, snapshots, and flat files. Runs never share state.
- **Tick**: the atomic unit of simulation time; carries an ordinal and a game timestamp.
- **Event**: an immutable record of something that happened, attributed to a tick,
  appended in order; the authoritative history of the run.
- **Snapshot**: a point-in-time capture of world state at a known tick, used to bound
  recovery time; always reconcilable with the event log.
- **Clock state**: current game time, speed setting, and pause flag; part of world state
  and durable across restarts.
- **Client session**: a transient attachment through which a viewer observes status and
  events and issues control commands; its lifecycle never affects the simulation's.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A world runs unattended for 24 real hours with no client attached, and its
  event record shows continuous coverage — no gaps, no restarts required.
- **SC-002**: A client attaching to a running world sees current world status within 2
  seconds, and its detach causes zero interruption to the simulation (verified by
  uninterrupted event ordinals across the detach).
- **SC-003**: After killing and restarting the daemon, the world resumes with 100% of
  recorded events intact and the game clock continuous with where it stopped; recovery
  completes in under 10 seconds for a run of at least one game day.
- **SC-004**: Pause takes effect within one tick; while paused, game time advances zero
  ticks over any real interval; resume continues from the exact paused point.
- **SC-005**: A speed change takes effect within one tick, and observed game-time
  progression matches the requested compression within 5% over a 5-minute measurement.
- **SC-006**: Two worlds created from the same seed and fed the same inputs produce
  identical event sequences (byte-for-byte equal histories), demonstrating determinism.

## Assumptions

- **Decided stack (from TASK-1, recorded here, elaborated in the plan phase)**: Go
  daemon, SQLite for the event log and snapshots, Bubble Tea TUI as the eventual client.
  The spec itself stays behavior-level; these choices bind the plan.
- **One daemon process manages one world run.** Multiple simultaneous worlds means
  multiple daemon processes, each with its own save directory. A registry of worlds
  beyond the save-directory convention is out of scope.
- **Local, single-operator deployment**: daemon and clients run on the same trusted host
  (the homelab MacBook); the client protocol needs no authentication in v1.
- **Simulation content is out of scope.** Agents, needs, maps, and LLM calls belong to
  later features (TASK-4..TASK-12). This substrate ships with a minimal placeholder
  simulation sufficient to generate real ticks and events, so the loop, log, snapshots,
  and protocol are exercised end-to-end.
- **Sequential attach is the required pattern**; concurrent multi-client attach may be
  refused in v1 (edge case above) as long as refusal is clean.
- **The event log is the source of truth**; snapshots are an optimization and may be
  discarded/rebuilt at the cost of longer replay.
- **Clock continuity means game-time continuity.** Real-world downtime while the daemon
  is stopped does not advance game time; the world sleeps in stasis, not in real time.
