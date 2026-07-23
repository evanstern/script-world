# Implementation Plan: Metatron Instruction Surface — Staged Charter + Skill Files + Gated Tool Roster

**Branch**: `task-64-metatron-instruction-surface` | **Date**: 2026-07-23 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/021-metatron-instruction-surface/spec.md`

## Summary

Grow the single player-editable `charter.md` into the full assistant-shaped configuration
surface: (1) a `skills/` folder of player-authored SKILL.md-style files composed into the
Metatron turn system prompt beneath the charter, per-read hot-reloaded like the charter;
(2) a per-world `capabilities.json` manifest that gates which acting tools (and which
miracle kinds) the world grants the angel — ungranted tools structurally absent from the
declared roster, the derived prose, AND refused at the door; (3) the hand-written prose
tool list in `turnSystemPrompt` replaced by text derived from the tool registry, with
per-kind miracle costs given one authoritative source in `internal/tool` from which both
`sim.miracleCost` enforcement and all prose derive; (4) `metatron.Status` extended with
per-file provenance and the granted tool set, rendered in the TUI console header.

## Technical Context

**Language/Version**: Go 1.26.4 (module `github.com/evanstern/promptworld`)

**Primary Dependencies**: stdlib + existing internal packages only — `internal/tool`
(leaf: registry/roster/derive), `internal/metatron` (charter, turn, status),
`internal/sim` (miracle cost enforcement; already imports `internal/tool`),
`internal/toolloop` (Job.Roster), `internal/ipc` (Status round-trip),
`internal/tui` (console header render). No new third-party deps.

**Storage**: plain files in the world save dir (`charter.md`, `skills/*.md`,
`capabilities.json`), read fresh per use — no watchers, no caching, no DB.

**Testing**: `go test ./...`; table-driven unit tests + adversarial fixture tests
(injection phrasings); existing drift-pinning test pattern (mirror tests) inverted into
single-source derivation tests.

**Target Platform**: macOS/Linux daemon + Bubble Tea TUI (unchanged)

**Project Type**: single Go module, multi-package CLI/daemon

**Performance Goals**: per-turn file reads (≤10 small files) are noise against an LLM
round-trip; no measurable turn-latency change.

**Constraints**: `internal/tool` stays a leaf (imports nothing internal);
determinism doctrine — identical world dir ⇒ byte-identical composed prompt;
replay unaffected (prompt composition feeds LLM calls, which are recorded inputs);
`.worktrees/` + one-task-one-PR discipline; TASK-63 concurrently edits
`internal/tui` (digest, villager detail, metatron transcript) — this feature confines its
TUI delta to the console status header (`consoleStatusMsg` handling + header render).

**Scale/Scope**: ~6 packages touched; est. net +600–900 LOC incl. tests; no schema/event
changes to the store (no new event types).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Artifact-Grounded Action** — PASS: spec/plan/research/contracts on disk; board
  TASK-64 In Progress with notes; decisions below cite code and prior specs.
- **II. One Task, One PR** — PASS: single branch `task-64-metatron-instruction-surface`
  in `.worktrees/task-64`, one PR; spec phases are internal breakdown.
- **III. Gates Over Assertions** — PASS: spec-bridge link before implementation; ACs
  ticked only against merged, test-proven artifacts.
- **IV. Grounding Freshness** — PASS (planned): touched sources are listed in
  `docs/wiki/` notes `metatron.md`, `metatron-miracles.md`, `tool-registry.md`,
  `tool-loop.md`, `tui-client.md`, `ipc-protocol.md` — wiki re-pin
  (`/grounding-wiki:wiki-update`) is an explicit task before Done.
- **V. Model-Tiered Workflow** — PASS: planning on Fable 5 (this document);
  implementation delegated to `spec-implementer`. **Tier: Opus 4.8.** Rubric
  justification: cross-package change (tool → sim cost-source inversion, metatron prompt
  assembly, ipc, tui) AND doctrine-adjacent behavior (persona-firewall-adjacent fixed
  frame; capability gating that must be structurally sound against prompt injection).
  Recorded on the board task.

**Post-Phase-1 re-check**: PASS — design adds no new packages, no new dependencies, no
violations; Complexity Tracking stays empty.

## Project Structure

### Documentation (this feature)

```text
specs/021-metatron-instruction-surface/
├── spec.md
├── checklists/requirements.md
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── instruction-surface.md    # file layout, composition, caps, notices
│   ├── capability-manifest.md    # capabilities.json shape + gating semantics
│   └── status.md                 # extended metatron.Status IPC shape
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created by plan)
```

### Source Code (repository root)

```text
internal/tool/
├── registry.go      # + MiracleCost per-kind table (single source); work_miracle unchanged shape
├── derive.go        # + RestrictEnum(tool, param, allowed); + MetatronToolGuidance(roster) prose derivation
└── (tests)          # derivation determinism, restriction, guidance-vs-registry drift tests

internal/sim/
└── miracles.go      # miracleCost now DERIVED from tool.MiracleCost (sim already imports tool)

internal/metatron/
├── charter.go       # + loadSkills (per-read, caps, notices); + loadManifest (grants, fallback+notice)
├── turn.go          # turnSystemPrompt(charter, skills, roster): fixed frame kept verbatim & last;
│                    #   prose tool list replaced by tool.MetatronToolGuidance(granted roster);
│                    #   Turn() builds granted roster per-read, passes to Job.Roster + handlers
├── toolcalls.go     # handlers map built from granted roster only
└── metatron.go      # Status gains Skills, GrantedTools, ManifestDefault (+ notices if needed)

internal/ipc/        # Status shape rides existing metatron.Status JSON — no protocol change
internal/tui/
└── tui.go           # consoleStatusMsg render: provenance summary + granted tool set in console header

e2e/ or internal/metatron tests
                     # adversarial fixture set (SC-002); no-manifest byte-compat test (SC-003)
```

**Structure Decision**: no new packages. The cost table and all derivation live in the
leaf `internal/tool`; `internal/metatron` composes per-read; `internal/sim` derives its
enforcement table from `internal/tool` (import direction already exists:
`sim/toolcheck.go` et al. import `internal/tool`). TUI delta is confined to the console
status header to minimize the TASK-63 collision surface.

## Complexity Tracking

No Constitution Check violations — table intentionally empty.
