# Implementation Plan: Metatron v1 — the editable angel

**Branch**: `task-12-metatron` | **Date**: 2026-07-20 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/005-metatron/spec.md`

## Summary

Metatron is a daemon-hosted gatekeeper agent (like the mind and scribe: a notify-fan-out
consumer with its own replica) that converses with the player over a new IPC request,
digests the event stream into an accreting notes file, and mediates nudges. Player text
enters only Metatron's prompt; villagers receive only Metatron's validated rendering as
high-salience memories, landed atomically through the existing `InjectSocial` whitelist
door. The charge economy is event-sourced into `sim.State` (regeneration emitted by the
executor as a pure function of state + tick; spends recorded by the nudge event), so
charges replay deterministically. The charter and Metatron's soul are flat files in the
save directory, loaded fresh each turn — a charter edit is live on the next turn by
construction. The TUI metatron pane becomes the console (transcript + input); a
`scriptworld metatron` one-shot CLI serves scripting and tests.

## Technical Context

**Language/Version**: Go 1.26 (existing module)

**Primary Dependencies**: Bubble Tea v1.3 + Lipgloss (TUI, already in use; add the
`bubbles` textinput companion if needed or hand-roll a minimal input), anthropic-sdk-go /
OpenAI-compatible HTTP (existing `internal/llm` orchestrator — no new transport),
modernc.org/sqlite (existing store)

**Storage**: existing per-world save dir — SQLite append-only event log (charge economy,
nudge events, moments) + flat files bound to the run (`charter.md`, `metatron/soul.md`,
`metatron/transcript.md`)

**Testing**: `go test ./... -race`; httptest mock providers for cloud calls; tmux-driven
live TUI checks; live proof on a real world (chronicle-proof, 14+ game days of history)

**Target Platform**: macOS/Linux daemon (homelab posture), terminal client

**Project Type**: single Go module — daemon + TUI client + CLI (existing layout)

**Performance Goals**: console turn ≤ 30 s (one cloud round-trip, SC-001); digests ≤ 4
cloud calls/game-day; zero impact on tick throughput (all Metatron I/O outside the loop)

**Constraints**: determinism contract untouchable (model output enters only as recorded
injected events); prompt-injection firewall is structural; cloud spend within the
existing $100/month ceiling ($ meter already enforces); world keeps running when cloud is
down

**Scale/Scope**: 8 villagers, 1 Metatron per world; ~4 digest + O(player) console calls
per game-day; charter ≤ 4,000 chars; notes file bounded by digest tail policy

## Constitution Check

*GATE: `.specify/memory/constitution.md` is an unfilled template — no formal
constitution exists. The project's standing rules (CLAUDE.md + grounded-assumptions)
serve as the gate:*

- **Determinism**: every world-state effect must be a recorded event through `Apply`;
  Metatron complies via `InjectSocial` (whitelisted `metatron.*` + `agent.memory_added`)
  and executor-emitted regeneration (pure function of state + tick). PASS
- **Artifact-grounded**: spec/plan/tasks under `specs/005-metatron/`, board task
  TASK-12 linked via spec-bridge, one branch/PR. PASS
- **LLM isolation**: all model traffic through `internal/llm`; new `KindMetatron`
  routes cloud; budget meter and circuit breaker apply unchanged. PASS
- **Files-bound-to-run**: charter/soul/transcript live inside the save dir, like
  personas and souls. PASS

*Post-design re-check (after Phase 1): no violations introduced — the design adds one
whitelisted event family, one IPC request, one LLM kind, and files inside the world dir.*

## Project Structure

### Documentation (this feature)

```text
specs/005-metatron/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── metatron-events.md     # event payloads + reducer effects
│   └── console-protocol.md    # IPC request/response + CLI + TUI console contract
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/metatron/       # NEW: the angel — replica, digests, moments, console turns,
│                        #   judgment/nudge pipeline, charter+soul file handling
├── metatron.go          #   component lifecycle, Observe, replica, turn serialization
├── digest.go            #   6-game-hour digest windows, moment triggers, notes file
├── turn.go              #   console turn: prompt build, judge/nudge parse, injection
├── charter.go           #   charter load/default/fallback, cap
└── *_test.go

internal/sim/
├── state.go             # State.MetatronCharges (+ genesis), Apply dispatch
├── metatron.go          # NEW: payload structs + reducer arms (nudged, regenerated)
├── executor.go          # charge regeneration emission at 6-game-hour boundaries
├── memory.go            # salDream constant (dream/omen memory salience)
└── loop.go              # whitelist: metatron.nudged, metatron.charge_regenerated*
                         #   (*regen is executor-emitted; only nudged needs the door)

internal/llm/llm.go      # KindMetatron → TierCloud

internal/ipc/            # metatron_chat request (long-call pattern like llm_call);
│                        #   status gains charges/metatron availability
internal/world/world.go  # CharterPath(), MetatronDir()
internal/persona/        # default charter text (authored constant), seeded by `new`
cmd/scriptworld/         # `scriptworld metatron <dir> [message]` one-shot console
internal/tui/            # metatron pane → console: transcript viewport + input line,
│                        #   charges display; tier health retained
internal/daemon/daemon.go# wire metatron component when LLM config exists
e2e/                     # determinism unaffected; console smoke via CLI
```

**Structure Decision**: mirror the established daemon-component pattern (`mind`,
`scribe`): `internal/metatron` is a notify consumer with its own replica, single-flight
workers, and file outputs — composed in `daemon.Run`, never called by other packages.

## Complexity Tracking

No constitution violations to justify. One deliberate simplicity choice: Metatron's soul
and transcript are plain files (not event-sourced) — they are Metatron's private memory
and the player's conversation record, not world state; world-visible effects (charges,
memories, moments) remain fully event-sourced.
