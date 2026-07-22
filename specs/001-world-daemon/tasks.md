# Tasks: World Daemon & Time Substrate

**Input**: Design documents from `/specs/001-world-daemon/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Included — the plan's Testing section and quickstart scenarios explicitly
require unit, integration, and e2e coverage (determinism and crash-resume are spec
success criteria, only provable by tests).

**Organization**: Tasks grouped by user story; each story phase is an independently
testable increment.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Go module and binary skeleton so everything after compiles and commits clean.

- [X] T001 Initialize Go module `github.com/evanstern/promptworld` (go.mod, Go 1.22+) at repo root; add `modernc.org/sqlite` dependency
- [X] T002 Create CLI skeleton in cmd/promptworld/main.go — subcommand dispatch table (new, daemon, start, stop, status, attach, tail, pause, resume, speed) with stub handlers returning "not implemented", usage text, exit-code discipline per contracts/cli.md
- [X] T003 [P] Extend .gitignore for Go build artifacts and test worlds (promptworld binary, /tmp test dirs are outside repo — ignore `/promptworld`, `dist/`)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The deterministic core every story stands on: save-dir layout, storage,
clock math, reducer, loop. No user story work before this completes.

- [X] T004 [P] Implement world package in internal/world/world.go — Manifest type (name, seed, created_at, format_version, tick_game_seconds), Create (refuse non-empty dir, write world.json, create agents/ dir), Open (validate manifest ↔ meta per contracts/storage.md), path helpers (DBPath, SockPath, PidPath, LogPath)
- [X] T005 [P] Implement store package in internal/store/schema.go + internal/store/store.go — open SQLite (WAL, synchronous=NORMAL) via modernc.org/sqlite, apply DDL from contracts/storage.md (events with append-only triggers, snapshots, meta), AppendEvents (one tx per tick batch, contiguous seq), ReplayEvents(sinceSeq) iterator, SaveSnapshot/LatestValidSnapshot (sha256 state_hash verify, fallback to older), PruneSnapshots(keep 24), meta get/set
- [X] T006 [P] Implement clock package in internal/clock/clock.go — Speed type (1x/4x/8x/16x/max), parse/format, tick↔game-time conversion (game epoch day 1 06:00, 1 tick = 1 game second), game-time display "day N HH:MM", interval-per-tick for a speed
- [X] T007 Implement deterministic sim state + reducer in internal/sim/state.go — State struct (clock state per data-model.md + placeholder sim state), Apply(event) reducer used identically live and in replay, canonical-JSON marshal (sorted keys) for snapshots and payloads, seeded math/rand/v2 PCG owned by State
- [X] T008 Implement placeholder simulation in internal/sim/placeholder.go — two wanderers on 16×16 grid, agent.moved each game-minute boundary via seeded RNG, sim.day_started/sim.night_started at 06:00/22:00, agent.slept at night start; ≤150 lines per plan scope guard
- [X] T009 Implement fixed-timestep loop in internal/sim/loop.go — single goroutine owning State; command intent queue applied only at tick boundaries with each applied command recorded as an event (clock.paused, clock.resumed, clock.speed_set); tick execution → events → store.AppendEvents → notify subscribers; pause skips tick advance; max speed spins with yield
- [X] T010 [P] Unit tests for clock math in internal/clock/clock_test.go — speed parse, tick↔game-time round trips, day/night boundaries, interval math for all speeds
- [X] T011 [P] Unit tests for store in internal/store/store_test.go — append-only triggers abort UPDATE/DELETE, seq contiguity, snapshot save/verify/fallback-on-corrupt, prune keeps events untouched
- [X] T012 Unit determinism harness in internal/sim/sim_test.go — two States, same seed, same tick-stamped command timeline, N=10,000 ticks: byte-identical event sequences and equal state hashes (SC-006 at package level); replay test: reduce logged events → same final state hash

**Checkpoint**: `go build ./... && go test ./...` green; foundation ready.

---

## Phase 3: User Story 1 — The world runs without me (Priority: P1) 🎯 MVP

**Goal**: A detached daemon advances the world 24/7; clients attach/detach freely
without touching the simulation.

**Independent Test**: quickstart Scenario A — new world, detached start, status shows
advancing tick at 4x, attach streams events, detach leaves it running.

### Implementation for User Story 1

- [X] T013 [P] [US1] Implement protocol types in internal/ipc/protocol.go — Request/Response/Push envelopes and status data shape exactly per contracts/client-protocol.md
- [X] T014 [US1] Implement IPC server in internal/ipc/server.go — UDS listener at world SockPath, JSON-lines framing, per-session goroutines, cmd dispatch (status, subscribe with since-replay then live, unsubscribe), bounded 1024-event push buffer with dropped push + subscription cancel on overflow, malformed-JSON closes connection, session death never touches the loop (FR-011)
- [X] T015 [US1] Implement daemon lifecycle in internal/daemon/daemon.go — wire world+store+loop+ipc; recovery-on-open (snapshot+replay via store/sim from T005/T007); pidfile write + stale pid/sock sweep; SIGTERM/SIGINT graceful path; daemon.started/daemon.stopped events
- [X] T016 [US1] Implement client in internal/ipc/client.go — connect, request/response correlation by id, push demux, used by CLI subcommands
- [X] T017 [US1] Wire CLI subcommands in cmd/promptworld/main.go — `new` (T004 Create + genesis world.created event), `daemon` (foreground primitive), `start` (re-exec detach, stdio→daemon.log, wait-for-socket ≤5s), `status` (attach-or-offline per contracts/cli.md), `attach` (status header + event stream + stdin commands), `tail` (--since/--follow, read-only when daemon down)
- [X] T018 [US1] Integration test in internal/ipc/ipc_test.go — in-process server+loop: attach, status <2s, subscribe from seq 0 gapless, abrupt client kill leaves loop ticking (FR-010, FR-011, SC-002)
- [X] T019 [US1] E2E test in e2e/daemon_e2e_test.go — build binary; Scenario A: new→start→status advancing→attach sees events→detach→status still advancing; event ordinals continuous across detach (SC-001 in miniature, SC-002)

**Checkpoint**: MVP — an always-on world you can watch and leave.

---

## Phase 4: User Story 2 — Time is a dial (Priority: P2)

**Goal**: Pause/resume as first-class verbs; runtime speed from 1x to max with honest
degradation.

**Independent Test**: quickstart Scenario B — pause freezes tick under real time
passing, resume continues exactly, speed changes hold their compression ratio.

### Implementation for User Story 2

- [X] T020 [US2] Add time-control commands through the loop in internal/sim/loop.go + internal/ipc/server.go — pause/resume/set_speed intents applied at tick boundary, recorded as clock.* events, idempotent semantics, response returns full status shape
- [X] T021 [US2] Implement auto-slow in internal/sim/loop.go — measure actual tick duration over 5s window; sustained overrun lowers effective_rate + emits clock.degraded, recovery climbs back + clock.recovered; game time only advances by executed ticks (FR-012, R7)
- [X] T022 [P] [US2] Wire CLI one-shots in cmd/promptworld/main.go — `pause`, `resume`, `speed <v>` printing resulting clock state per contracts/cli.md; add same commands to `attach` stdin loop
- [X] T023 [US2] E2E test in e2e/daemon_e2e_test.go — Scenario B: pause → tick frozen across 2s real sleep → resume continues from exact tick; speed 1x vs 4x compression ratio within 5% over measurement window; detach-is-not-pause re-verified (SC-004, SC-005)

**Checkpoint**: US1 + US2 — ambient world with the time dial in hand.

---

## Phase 5: User Story 3 — Nothing is ever lost (Priority: P3)

**Goal**: Kill-proof persistence: snapshot cadence, lossless resume, clean stop,
separable save dirs.

**Independent Test**: quickstart Scenario C — kill -9 mid-run, restart, clock/speed/
pause state and full history intact; Scenario E — copied dir is a runnable world.

### Implementation for User Story 3

- [X] T024 [US3] Implement snapshot cadence + shutdown path in internal/daemon/daemon.go + internal/sim/loop.go — snapshot every 3600 ticks, on pause, on graceful shutdown; `shutdown` IPC cmd + `stop` CLI subcommand in cmd/promptworld/main.go (idempotent when not running); restart-while-paused wakes paused
- [X] T025 [US3] Harden recovery in internal/daemon/daemon.go + internal/store/store.go — corrupt-snapshot fallback chain to genesis, seq gap check fatal with guidance (contracts/storage.md recovery procedure), recovery timing surfaced in daemon.started payload
- [X] T026 [US3] E2E tests in e2e/daemon_e2e_test.go — Scenario C: kill -9, restart, zero event loss, clock continuity, <10s recovery after ≥1 game-day at max speed (SC-003); restart-while-paused case; Scenario E: stop, cp -R, start the copy (FR-009)
- [X] T027 [P] [US3] Full-binary determinism e2e in e2e/determinism_e2e_test.go — two worlds, same seed, identical command timeline at max speed, N ticks: `SELECT seq,tick,type,payload FROM events` byte-identical (SC-006, quickstart Scenario D)

**Checkpoint**: All three stories independently verified.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T028 [P] Run full quickstart.md scenarios manually against the built binary; fix any drift between contracts and behavior; record results in specs/001-world-daemon/quickstart-results.md
- [X] T029 [P] Update README.md with build/run instructions for the daemon substrate (replacing pitch-only content where it claims features that now exist)
- [X] T030 `go vet ./...` clean; `gofmt -l .` empty; race detector pass on test suite (`go test -race ./...`)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)** → nothing
- **Foundational (Phase 2)** → Setup; **blocks all stories**
- **US1 (Phase 3)** → Foundational
- **US2 (Phase 4)** → Foundational + loop/IPC pieces of US1 (T014, T015)
- **US3 (Phase 5)** → Foundational + daemon lifecycle of US1 (T015)
- **Polish (Phase 6)** → all stories

### Parallel Opportunities

- T004, T005, T006 (different packages) after T001–T002
- T010, T011 alongside T007–T009
- T013 alongside T015; T022 alongside T023's authoring; T027 alongside T026
- US2 and US3 implementation can proceed in parallel once T014/T015 exist

---

## Implementation Strategy

MVP = Phases 1–3 (US1): an always-on world you can attach to. US2 then adds the time
dial, US3 the durability contract. Each checkpoint runs its story's quickstart scenario
before moving on; commit per task or coherent group on branch `001-world-daemon`
(one TASK, one PR — this whole tasks.md lands as TASK-2's single PR).
