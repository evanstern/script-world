# Tasks: World Instance Manager

**Input**: Design documents from `/specs/008-instance-manager/`

**Prerequisites**: plan.md, spec.md, research.md (D1–D7), data-model.md, contracts/cli.md, quickstart.md

**Tests**: Included — the spec's SC-003 explicitly requires the existing regression/e2e
suite passing **plus new name-resolution tests**, and this repo's convention is tests
alongside code (`*_test.go`) plus black-box e2e in `e2e/`.

**Organization**: Grouped by user story; US1 (`ps`) is the MVP slice.

## Format: `[ID] [P?] [Story] Description`

## Path Conventions

Single Go module at repo root: `cmd/scriptworld/`, `internal/`, `e2e/` (per plan.md).

---

## Phase 1: Setup

**Purpose**: Branch/worktree per constitution Principle II; no scaffolding needed (existing module).

- [X] T001 Create worktree `.worktrees/task-43` on branch `task-43-instance-manager` from fresh `origin/main` (`git fetch origin && git worktree add .worktrees/task-43 -b task-43-instance-manager origin/main`); confirm `go build ./...` and `go test ./...` are green at baseline

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The `internal/worlds` package core — home paths, registry, name rules, discovery — that every story consumes.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T002 [P] Create `internal/worlds/home.go`: `Root()` (`$SCRIPTWORLD_HOME` else `~/.scriptworld`), `WorldsHome()` (`<root>/worlds`), `RegistryPath()` (`<root>/known_worlds.json`), `ValidateName(name)` per data-model.md rules (non-empty, no `/`, no leading `-` or `.`); unit tests in `internal/worlds/home_test.go` covering override env var and every validation rule
- [X] T003 [P] Create `internal/worlds/registry.go`: registry file shape `{"worlds":{name:{path}}}` per data-model.md; `LoadRegistry()` tolerant of missing/corrupt file (⇒ empty, never error), `Upsert(name, path)` atomic (temp+rename, prunes entries whose dir lacks readable `world.json` and entries whose path is inside the current worlds home); unit tests in `internal/worlds/registry_test.go` including corrupt-file, prune-on-write, and moved-world upsert-by-name repair
- [X] T004 Create `internal/worlds/resolve.go`: `IsPathArg(arg)` (contains `/` or leading `.`/`~` — D3), `Resolve(arg)` returning the world dir with worlds-home-first order, `ErrAmbiguous` listing both candidate paths, not-found error naming the searched worlds home and suggesting `scriptworld ps --all` (FR-007/FR-011); unit tests in `internal/worlds/resolve_test.go` for the full decision table in data-model.md (depends on T002, T003)
- [X] T005 Create `internal/worlds/discover.go`: `Discover()` returning deduped candidates = worlds-home scan (immediate subdirs with `world.json`; unreadable manifests flagged, never fatal) ∪ registry entries (missing dirs flagged); unit tests in `internal/worlds/discover_test.go` (depends on T002, T003)

**Checkpoint**: `go test ./internal/worlds/` green — user story phases can begin.

---

## Phase 3: User Story 1 — See everything that is running (Priority: P1) 🎯 MVP

**Goal**: `scriptworld ps [--all] [--json]` lists every world machine-wide with live-proven state; stale leftovers never show as running.

**Independent Test**: Start two worlds by path (existing commands), run `scriptworld ps` from an unrelated directory → both listed with live data; SIGKILL one → it no longer shows as running; nothing running → empty listing, exit 0 (spec US1 Independent Test).

### Implementation for User Story 1

- [X] T006 [P] [US1] Create `internal/worlds/probe.go`: per-candidate classification per the data-model.md state machine (`running|paused|unresponsive|stopped|missing|unreadable`) — pidfile pre-filter (reuse `daemon.IsRunning`), bounded dial+`status` call (~1s per-world budget) run concurrently across candidates (D2, SC-001 < 2s), offline last-known clock via store snapshot + last event tick (extract/share the logic currently in `cmdStatus`'s offline branch, `cmd/scriptworld/commands.go:320-346`); unit tests in `internal/worlds/probe_test.go` for classification (fake pidfiles/sockets)
- [X] T007 [US1] Register worlds on daemon boot: in `internal/daemon/daemon.go` `Run()`, after `world.Open`/pidfile acquisition, best-effort `worlds.Upsert(manifest name, dir)` iff dir is outside the current worlds home — failure logs and continues, never fatal (D1/D6, FR-008) (depends on T003)
- [X] T008 [US1] Create `cmd/scriptworld/ps.go`: `cmdPs` — discovery (T005) + probe (T006), human table per contracts/cli.md (`NAME STATE PID TICK GAME TIME SPEED LLM PATH`, `no worlds running` when empty, exit 0), `--all` adds non-live states, `--json` array reusing `status --json` vocabulary + `name`/`path`/`state` (`llm` presence ⇒ on; stopped worlds get `llm_configured` from `llm.json` existence); output-shaping tests in `cmd/scriptworld/ps_test.go` (depends on T006)
- [X] T009 [US1] Wire `ps` into `cmd/scriptworld/main.go`: dispatch case + usage text line (`scriptworld ps [--all] [--json]`) (depends on T008)
- [X] T010 [US1] e2e in `e2e/manager_e2e_test.go` (pattern from `e2e/daemon_e2e_test.go`, isolated `SCRIPTWORLD_HOME` per test): two daemons started by path appear in `ps` from an unrelated CWD with name/state/pid/tick/time/speed/LLM; SIGKILL one → not running in next `ps`; empty home → "no worlds running" exit 0; `ps --json | len == 2`; wedged-daemon timeout budget respected (listing < 2s) (depends on T007, T009)

**Checkpoint**: US1 fully functional — `ps` answers "who is clobbering the LLM?" (SC-001, SC-005).

---

## Phase 4: User Story 2 — Create and address worlds by name (Priority: P2)

**Goal**: `new <name>` creates in the worlds home; every per-world command accepts a name or a path; path invocations byte-compatible.

**Independent Test**: `scriptworld new testworld` → complete world under the worlds home; `start`/`status`/`stop testworld` from another directory resolve to it; duplicate `new testworld` refused untouched (spec US2 Independent Test).

### Implementation for User Story 2

- [X] T011 [US2] Rework `cmdNew` in `cmd/scriptworld/commands.go` per contracts/cli.md: bare-word arg = name-form (create `<worlds-home>/<name>` with manifest name = arg, lazily create worlds home, refuse existing dir exit 1 untouched, reject `--name`, validate via `worlds.ValidateName`); path-shaped arg = legacy form verbatim (`--name`/basename, reject `--at`); `--at DIR` = create at exactly DIR with manifest name `<name>` + registry Upsert; success output suggests name-based `start` for name-form; unit tests in `cmd/scriptworld/commands_test.go` for all three forms + validation failures (depends on T002–T004)
- [X] T012 [US2] Thread name-or-path resolution through every per-world command: add a shared `resolveWorld(arg)` helper in `cmd/scriptworld/commands.go` (path-shaped → unchanged today's behavior; else `worlds.Resolve`) and apply it in `dirArg`/`parseDirFlags` call sites for `daemon`, `start`, `stop`, `status`, `pause`, `resume`, `speed`, `ui`, `attach`, `tail`, `metatron`, `llm` (`cmd/scriptworld/commands.go`) and `calibrate` (`cmd/scriptworld/calibrate.go`); update usage text in `cmd/scriptworld/main.go` (`<dir>` → `<world>`, new `new` synopsis); resolution-plumbing tests in `cmd/scriptworld/commands_test.go` (depends on T004)
- [X] T013 [US2] e2e in `e2e/manager_e2e_test.go`: full name lifecycle from an unrelated CWD — `new aria` → dir + manifest under home; duplicate refused exit 1; invalid names (`-flag`, empty, and a `/`-bearing value via `--name 'bad/name'` — a bare `bad/name` argument is a *path* per D3 and correctly takes the legacy path-form) exit 1; `start`/`status`/`speed`/`pause`/`resume`/`stop aria`; unknown name exit 1 with worlds-home named and `ps --all` suggested; `stop aria` idempotent exit 0; copied world dir starts under a fresh `SCRIPTWORLD_HOME` with zero manager state (SC-004) (depends on T011, T012)

**Checkpoint**: US1 + US2 independently green; SC-002 (zero paths typed) demonstrable.

---

## Phase 5: User Story 3 — Manage custom-path worlds by name (Priority: P3)

**Goal**: A custom-path world started once by path is thereafter addressable by name; registry lies are healed, never fatal.

**Independent Test**: Create+start a world at a custom path → `ps` shows it by name, `stop <name>` works; record survives to `ps --all`; deleting the directory makes the manager forget it gracefully (spec US3 Independent Test).

### Implementation for User Story 3

- [X] T014 [US3] Harden resolution/listing for registry drift in `internal/worlds/resolve.go` + `internal/worlds/probe.go`: name → registry entry whose dir is gone resolves to a "missing" error (exit 1, helpful message, never a raw open error); ambiguous home+registry collision refused with both paths (FR-011); `ps --all` renders `missing` and `unreadable` rows per contracts/cli.md without aborting the listing; unit tests in `internal/worlds/resolve_test.go` and `internal/worlds/probe_test.go` (depends on T004–T006)
- [X] T015 [US3] e2e in `e2e/manager_e2e_test.go`: custom-path world started by path appears in `ps` under its manifest name (registered by daemon boot, T007); `stop <name>` stops it; survives to `ps --all` stopped; `rm -rf` the dir → `ps --all` shows missing (or omits) and by-name commands exit 1 gracefully; home/registry name collision → ambiguous refusal listing both paths (depends on T007, T012, T014)

**Checkpoint**: All three stories independently functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T016 [P] Reconcile user-facing docs: README.md command examples and `specs/001-world-daemon/contracts/cli.md` cross-reference note (base contract now extended by `specs/008-instance-manager/contracts/cli.md`); ensure `scriptworld help` usage text matches contracts/cli.md exactly
- [ ] T017 Run the full quickstart.md walkthrough against a fresh build (hermetic `SCRIPTWORLD_HOME`) and the complete suite `go test ./... && go test ./e2e/`; fix anything it surfaces; record the run in the PR description

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (P1)** → **Foundational (P2)** → user stories.
- **US1 (Phase 3)**: needs T002–T005 (+T003 for the daemon hook). Independent of US2/US3.
- **US2 (Phase 4)**: needs T002–T004. Independent of US1 (resolution doesn't need `ps`).
- **US3 (Phase 5)**: needs US1's T007 (daemon registration) and US2's T012 (name plumbing) for its e2e; T014 itself only needs Foundational + T006.
- **Polish (Phase 6)**: after desired stories.

### Within-story ordering

T006 → T008 → T009 → T010; T011/T012 parallelizable after Foundational, then T013; T014 → T015.

### Parallel Opportunities

- T002 ∥ T003 (different files, no deps).
- After Foundational: T006 ∥ T007 ∥ T011 ∥ T012 (different files); US1 and US2 phases can proceed concurrently.
- T016 ∥ anything after stories complete.

### Post-merge obligations (outside tasks.md, per constitution)

- `/grounding-wiki:wiki-update` — this feature touches wiki-pinned sources
  (`cmd/scriptworld/*`, `internal/daemon/daemon.go`); TASK-43 is not Done until re-pinned.
- `spec-bridge:sync` after each phase lands to keep the board honest.

---

## Implementation Strategy

**MVP first**: T001–T010 ship `ps` alone — the core pain ("what's running / who's
clobbering the LLM") is answered with zero addressing changes. Then US2 (names), then
US3 (custom-path convenience). All phases land as commits on the single
`task-43-instance-manager` branch and merge in TASK-43's one PR.
