# Implementation Plan: Calibration UX — uncalibrated worlds warn instead of silently over-suppressing

**Branch**: `task-40-calibration-ux` | **Date**: 2026-07-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/035-calibration-ux/spec.md`

## Summary

Pure visibility slice, no behavior change (spec FR-007): an uncalibrated LLM world must be
impossible to run at a suppressing speed without knowing it. Four surfaces: (1) the daemon boot
line for a missing/unreadable calibration profile becomes a warning carrying the per-class
suppression horizon at bootstrap seeds plus the exact calibrate command; (2) `set_speed` replies
gain an additive warning when a bootstrap-seeded provider's class is suppressed at the new speed
— computed with the router's own arithmetic and current estimates, never blocking the speed
change; (3) status gains an additive per-provider `calibrated_at` field (empty = bootstrap) so
uncalibrated state outlives the boot scroll; (4) `promptworld calibrate` output discloses its
sequential-measurement floor. The horizon arithmetic that `cmd/promptworld/calibrate.go` already
implements (`horizonSummary`) moves into `internal/cognition` as the shared, exported home.

## Technical Context

**Language/Version**: Go 1.x (existing module `github.com/evanstern/promptworld`)

**Primary Dependencies**: stdlib only in `internal/cognition` (leaf-purity doctrine, doc.go);
existing internal packages elsewhere (`internal/llm`, `internal/ipc`, `internal/daemon`,
`cmd/promptworld`). No new external dependencies.

**Storage**: none new — reads the existing `calibration.json` load outcome; never writes it.

**Testing**: `go test ./...`; table/behavior tests alongside packages. Precedents:
`internal/ipc/ipc_test.go` (set_speed reply assertions, incl. the max-gate), `cmd/promptworld/calibrate_test.go`
(output shape), `internal/llm` orchestrator tests (SeedCalibration/StatusSnapshot).

**Target Platform**: darwin/linux daemon + CLI (unchanged)

**Project Type**: single Go module — daemon + CLI + TUI client

**Performance Goals**: none — warning computation is O(#classes × #ladder notches) integer/float
arithmetic on an existing request path; negligible.

**Constraints**: `internal/cognition` stays a stdlib-only leaf (no llm/ipc/mind imports).
Additive-omitempty wire fields (spec FR-008): no-LLM worlds byte-identical everywhere; calibrated
worlds byte-identical on boot + speed-change surfaces. Zero routing/estimator behavior change
(spec FR-007, SC-003).

**Scale/Scope**: ~6 files touched + tests; no schema/persistence changes; no new packages.

## Constitution Check

*GATE: evaluated against constitution v1.1.0 before Phase 0; re-checked after Phase 1.*

- **I. Artifact-Grounded Action** ✔ — spec 035 + this plan are the artifacts; the TASK-40
  "revisit bootstrap default" question is closed by the spec's Doctrine Review section, not by
  chat. Board linked via spec-bridge (TASK-40, In Progress).
- **II. One Task, One PR** ✔ — TASK-40 ↔ one branch (`task-40-calibration-ux` in
  `.worktrees/task-40`) ↔ one PR. Spec docs commit to main at root (project convention).
- **III. Gates Over Assertions** ✔ — spec-bridge gate active; phase ACs mirrored from tasks.md
  once generated; status never exceeds artifacts.
- **IV. Grounding Freshness** ✔ — plan includes a polish-phase wiki re-pin
  (`docs/wiki/cognition.md` names estimate.go/daemon boot line among sources; ipc/status notes
  likely touched) + player-docs freshness check.
- **V. Model-Tiered Workflow** ✔ — this plan is planning-tier output; implementation delegates
  to `spec-implementer`. Tier call: **Sonnet (default)** — single-purpose additive UX fields,
  view/CLI rendering, tests alongside; no concurrency/scheduling logic is modified. The two
  `internal/llm` touches (retain `calibratedAt` in SeedCalibration; copy it into StatusSnapshot)
  are additive bookkeeping on existing structs, not orchestration changes — rubric's
  "concurrency/scheduling/governor logic" clause is not met. Escalate to Opus only if gates fail.

**Post-Phase-1 re-check** ✔ — design added no new packages, no doctrine changes, no violations.
Complexity Tracking stays empty.

## Project Structure

### Documentation (this feature)

```text
specs/035-calibration-ux/
├── spec.md
├── plan.md              # This file
├── research.md          # Phase 0 — decisions & alternatives
├── data-model.md        # Phase 1 — seed-state + summary entities
├── quickstart.md        # Phase 1 — end-to-end validation guide
├── contracts/
│   └── warnings.md      # Phase 1 — boot line, set_speed reply, status field, calibrate disclosure
├── checklists/requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
internal/cognition/
├── estimate.go          # UNCHANGED constants (doctrine) — referenced only
├── horizon.go           # NEW: exported HorizonSummary + per-class suppression helpers
│                        #   (lifted from cmd/promptworld/calibrate.go horizonSummary; stdlib-only)
└── horizon_test.go      # NEW: table tests at bootstrap + calibrated values

internal/llm/
└── llm.go               # provider gains calibratedAt (set in SeedCalibration from profile
                         #   entry presence); ProviderStatus += CalibratedAt omitempty;
                         #   Orchestrator exposes seed-state read (for ipc warning gate)

internal/ipc/
├── protocol.go          # StatusData += Warning string `json:"warning,omitempty"`
├── server.go            # set_speed: after validation, before reply — compose warning via
│                        #   cognition horizon arithmetic + orchestrator seed state (non-blocking)
└── ipc_test.go          # set_speed warning present/absent scenarios (US1 1–4)

internal/daemon/
└── daemon.go            # boot: no-profile and unreadable-profile branches print the
                         #   suppression-horizon warning + calibrate command (US2)

cmd/promptworld/
├── calibrate.go         # horizonSummary delegates to cognition.HorizonSummary; sequential-floor
│                        #   disclosure printed adjacent to results + horizon summary (US4)
├── calibrate_test.go    # disclosure assertions
├── commands.go          # status + set-speed rendering: print warning field; per-provider
│                        #   calibrated_at / "uncalibrated (bootstrap)" (US3)
└── commands_test.go     # rendering tests
```

**Structure Decision**: no new packages. The one structural move is lifting the horizon
arithmetic into `internal/cognition/horizon.go` so daemon, ipc, and calibrate all read one
implementation (spec FR-006: the warning may never disagree with the router — sharing the code
makes that structural). `internal/ipc` already imports `internal/llm`; adding the
`internal/cognition` import keeps the dependency direction toward leaves.

## Complexity Tracking

No constitution violations — table intentionally empty.
