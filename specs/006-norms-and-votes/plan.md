# Implementation Plan: Norms and Votes

**Branch**: `task-13-norms-and-votes` | **Date**: 2026-07-20 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/006-norms-and-votes/spec.md`

## Summary

Governance is deterministic sim, not model cognition. A new `internal/sim/governance.go`
owns the whole meeting lifecycle as pure executor-emitted events: a convening beat before
noon pins awake villagers to an event-sourced meeting place, the meeting opens at noon
(SecondOfDay 43200), speaking turns fire on a fixed cadence inside a 3600-tick timebox
(+bounded grace), proposals are tabled deterministically from standing fodder (gru
attacks → curfew, broken debts → repay-debts, village-wide hostility → exile, own
violations → amend/repeal), votes resolve per-attendee as a pure integer function of
Relation edges, and passed norms land in event-sourced `State.Norms`. The language model
never decides an outcome: an optional meeting driver in `internal/mind` (convo-driver
pattern) only rephrases proposal text, injected through the `InjectSocial` whitelist as a
recorded event. Norms enter planner context via a new prompt section; violations are
detected deterministically (curfew sweep, promise_broken piggyback, exile proximity) and
land witness memories, edge movement, and rumor fodder through existing machinery. The
village charter is a scribe-rendered flat file (`village_charter.md`) — a derived view of
event-sourced norm state, so it replays and survives restarts by construction, and never
collides with Metatron's player-editable `charter.md`.

## Technical Context

**Language/Version**: Go 1.26 (existing module; no new dependencies)

**Primary Dependencies**: existing internal packages only — `internal/sim` (reducer +
executor), `internal/mind` (planner prompt, optional phrasing driver), `internal/llm`
(orchestrator, local tier), `internal/scribe` (charter render), `internal/store`
(event log), modernc.org/sqlite (existing)

**Storage**: existing per-world save dir — governance state fully event-sourced in the
SQLite log + snapshots; `village_charter.md` is a regenerable scribe render at world root

**Testing**: `go test ./... -race`; table-driven reducer/executor tests via the
`driveTicks` harness (`internal/sim/governance_test.go`); mocked-Submitter driver tests
(`internal/mind/meeting_test.go`); determinism e2e replay extension

**Target Platform**: macOS/Linux daemon (homelab posture), terminal client

**Project Type**: single Go module — daemon + TUI client + CLI (existing layout)

**Performance Goals**: zero tick-throughput impact (all governance beats are O(agents)
integer math on scheduled ticks); ≤ ~10 governance events per meeting turn; optional
phrasing calls ≤ 1 per tabled proposal (local tier, best-effort)

**Constraints**: determinism contract untouchable — reducer never calls a model;
executor governance beats are pure functions of (state, map, tick); model output enters
only as whitelisted injected events (phrasing, never outcomes); governance never stalls
on inference (degraded mode = template text)

**Scale/Scope**: 8 villagers, 1 meeting/game-day, ≤ 8 speaking turns/meeting, norms
list expected single-digit; 3 checkable norm kinds + exile in v1

## Constitution Check

*GATE: `.specify/memory/constitution.md` is an unfilled template — no formal
constitution exists. The project's standing rules (CLAUDE.md + grounded-assumptions)
serve as the gate:*

- **Determinism**: every governance effect is a recorded event through `Apply`; meeting
  lifecycle and votes are executor-emitted pure functions of (state, map, tick); the
  only injected governance event is text rephrasing, whitelisted and dry-run-validated.
  Replay reproduces charter, tallies, and edges with zero model calls. PASS
- **Artifact-grounded**: spec/plan/tasks under `specs/006-norms-and-votes/`, board task
  TASK-13 linked via spec-bridge, one branch/PR. PASS
- **LLM isolation**: phrasing goes through `internal/llm` on the local tier,
  best-effort; failure means template text stands. No new transport. PASS
- **Files-bound-to-run**: `village_charter.md` lives in the save dir as a derived
  render (like soul.md/chronicle.md), never authoritative, never global. PASS

*Post-design re-check (after Phase 1): no violations introduced — the design adds one
event domain (`meeting.*` / `norm.*`), one whitelisted injectable type, one optional
LLM call kind, state fields on `sim.State`, and one derived file in the world dir.*

## Project Structure

### Documentation (this feature)

```text
specs/006-norms-and-votes/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── governance-events.md   # event payloads + reducer effects + whitelist delta
│   └── meeting-lifecycle.md   # schedule, phases, vote function, fodder rules,
│                              #   planner-context block, charter render contract
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/sim/
├── state.go             # State: +MeetingPlace, +Meeting, +Norms, +NextNormID,
│                        #   +NextProposalID; Apply dispatch: meeting.*/norm.* →
│                        #   applyGovernance
├── governance.go        # NEW: Norm/MeetingState/payload structs, tuning consts,
│                        #   applyGovernance reducer arm, vote function
│                        #   (pure fn of Relation edges), fodder rules, violation
│                        #   detection helpers, meeting-place derivation
├── governance_test.go   # NEW: table-driven lifecycle/vote/violation/replay tests
├── executor.go          # convene beat (SecondOfDay 41400), open at noon (43200),
│                        #   turn cadence, timebox+grace close, attendee pinning,
│                        #   curfew sweep (piggybacks the per-minute needs beat),
│                        #   exile proximity check, promise_broken norm piggyback
├── loop.go              # injectSocialWhitelist += meeting.proposal_rephrased
└── memory.go            # salience constants for meeting/violation memories

internal/mind/
├── prompt.go            # planner context: "Village law" section (active norms,
│                        #   exile judgments, meeting-at-noon line)
├── meeting.go           # NEW (enrichment only): observes meeting.proposal_tabled,
│                        #   one best-effort local call to rephrase template text,
│                        #   injects meeting.proposal_rephrased; single-flight
├── meeting_test.go      # NEW: mocked Submitter; skip-on-failure semantics
├── narrate.go           # chronicleNote cases: meeting opened/closed, proposal
│                        #   tabled/resolved, norm enacted/amended/repealed,
│                        #   violation witnessed, exile passed (agents named)
└── mind.go              # absorb: suppress planning for attendees while meeting open

internal/llm/llm.go      # KindMeeting → TierLocal (best-effort phrasing)

internal/scribe/scribe.go# render village_charter.md from replica Norms on
│                        #   governance-event dirty marks; render-on-start
internal/world/world.go  # VillageCharterPath()
internal/tui/            # (small) chronicle already displays; no new pane
e2e/                     # determinism e2e: governance events replay to same hash
```

**Structure Decision**: governance core is deterministic sim (`internal/sim/
governance.go`), following the gru/consolidation precedent — executor-driven, not a
daemon component, because meetings must run with no LLM configured. The only
model-facing piece (proposal phrasing) lives in `internal/mind` beside the convo
driver, since it is villager-voiced cognition and reuses the mind's replica, absorb
loop, and injector seams. The charter render lives in `internal/scribe`, the
established owner of derived flat-file views.

## Complexity Tracking

No constitution violations to justify. Two deliberate simplicity choices:
1. **Proposals resolve in the beat they are tabled** — no open-proposal state
   persists across ticks; a Proposal is an event-payload lifecycle, not a State
   entity. Rejected alternative (multi-day open proposals with a voting window)
   added ledger-style state and sweep machinery for no v1 story value.
2. **Norm kinds are a closed vocabulary** (curfew / repay_debts / exile) — free-text
   norms cannot be violation-checked deterministically. The model may phrase them;
   the sim enforces only what it can observe. Post-v1 kinds extend the enum.
