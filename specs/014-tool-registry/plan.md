# Implementation Plan: Tool Registry — single source of truth for agent capabilities (Layer 1)

**Branch**: `014-tool-registry` | **Date**: 2026-07-22 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/014-tool-registry/spec.md` (clarified 2026-07-22)

## Summary

Replace the four hand-maintained capability vocabularies (prompt string, mind parse map,
sim plan-step map, plus per-verb switch arms) with one leaf registry package
(`internal/tool`) from which the prompt vocabulary, mind-side validation, and sim-door
validation are derived; express capability as per-agent-kind roster membership; migrate
all existing capabilities (19 world verbs, say, muse, gist, metatron converse + nudges)
onto registry entries. Behavior- and replay-identical except one documented delta: the
plan-step map's shipped drift (TASK-55, spec 012 FR-020 violation) is cured by
derivation. Declarative data moves to the registry; gates, doors, executors, and
reducers stay exactly where they are (the gate decides; the model never asserts
outcomes). Full decisions: [research.md](research.md) R1–R9.

## Technical Context

**Language/Version**: Go 1.26 (module `github.com/evanstern/promptworld`)

**Primary Dependencies**: none new — stdlib only; new package `internal/tool` is an
internal leaf (imports nothing internal, R1), consumed by `internal/sim`,
`internal/mind`, `internal/metatron`, `internal/daemon`

**Storage**: none — registry/rosters are static compiled data; zero event-log or
snapshot format changes (replay untouched by construction, verified by suite)

**Testing**: `go test ./...`; new golden-prompt fixture (captured pre-refactor),
single-walk invariant test, door-equivalence table test, startup-validation tests;
existing replay byte-identity suite must pass unmodified

**Target Platform**: same as project (macOS/Linux daemon + CLI/TUI)

**Project Type**: single Go module, internal refactor layer

**Performance Goals**: nil deltas — derivation runs at init (one registry walk);
per-tick and per-injection hot paths keep map-lookup shape

**Constraints**: byte-identical prompts (SC-003); replay byte-identity (SC-002);
`injectSocialWhitelist` diff empty (FR-013); landing ladder untouched (FR-014); the
FR-012 drift cure is the sole behavioral delta

**Scale/Scope**: ~25 registry entries (19+ world verbs — final count set by post-TASK-51
main — 3 expressive, 3 metatron); touches `internal/tool` (new), `internal/mind`
(prompt.go, parse.go), `internal/sim` (loop.go, plan.go, agents.go, metatron.go),
`internal/metatron` (turn.go), `internal/daemon` (startup validation)

## Constitution Check

*GATE: evaluated pre-Phase-0 and re-checked post-Phase-1 (v1.1.0).*

- **I. Artifact-grounded action** — PASS. Spec linked to TASK-53 (spec-bridge, In
  Progress); drift defect filed as TASK-55 before planning proceeded; every clarify
  answer recorded in spec.md; this plan + research/data-model/contracts/quickstart are
  the decision artifacts.
- **II. One task, one PR** — PASS. TASK-53 = one branch (`.worktrees/task-53`, cut from
  post-TASK-51 main per clarified sequencing) = one PR. Spec phases are internal
  breakdown. TASK-55 closes inside this same PR by explicit clarification (its AC is
  FR-012's delta), noted on both tasks — not a second PR.
- **III. Gates over assertions** — PASS. Board status driven by spec-bridge sync;
  quickstart.md defines the physical evidence (green suites, grep-empty duplicate maps,
  golden fixture) behind each phase claim.
- **IV. Grounding freshness** — PASS with obligation. Touched wiki sources: agent-mind,
  sim-loop, reflex-policy, executor, metatron, cognition, event-types. PR is not done
  until `/grounding-wiki:wiki-update` re-pins them (tracked as a task in tasks.md).
- **V. Model-tiered workflow** — PASS. Planning on Fable 5 (this document).
  Implementation tier: **Opus 4.8** for the registry package + door/derivation slices —
  rubric: cross-package architectural change touching `internal/sim` injection
  validation and `internal/mind` orchestration-adjacent code, with a replay-identity
  guarantee at stake. Sonnet for mechanical slices (cap-literal moves, doc/wiki prep).
  Tier choice + rubric recorded on TASK-53 at implementation kickoff. No inline
  implementation by the planning model.
- **Spec rigor** — PASS. specify → clarify (3 questions, 2026-07-22) → this plan →
  tasks (next) → implement, in order.

**Post-Phase-1 re-check** (after research.md, data-model.md, contracts/, quickstart.md):
no new violations. The one scope-shaped decision — restructuring `resolveGoal`/
`intentDuration` into name-keyed tables while leaving executor/reducer arms alone
(R2) — is the *simpler* alternative, chosen precisely to protect FR-011; no Complexity
Tracking entries needed.

## Project Structure

### Documentation (this feature)

```text
specs/014-tool-registry/
├── spec.md              # clarified specification
├── plan.md              # this file
├── research.md          # R1–R9 decisions (Phase 0)
├── data-model.md        # Tool/EffectClass/Param/Gate/Cost/Roster + derived surfaces (Phase 1)
├── quickstart.md        # end-to-end validation guide (Phase 1)
├── checklists/requirements.md
├── contracts/
│   ├── tool-catalog.md  # the complete entry enumeration (migration contract)
│   └── registry-api.md  # Go API surface + consumption points + test contract
└── tasks.md             # Phase 2 (/speckit-tasks — next)
```

### Source Code (repository root)

```text
internal/
├── tool/                    # NEW leaf package (R1)
│   ├── tool.go              # Tool, EffectClass, Param, GateClass, Cost types
│   ├── registry.go          # entries (catalog order), All/Lookup, derived-surface funcs
│   ├── roster.go            # RosterVillager, RosterMetatron, OnRoster
│   ├── validate.go          # Validate() — R9 checks
│   └── *_test.go            # validation, single-walk invariant, catalog completeness
├── mind/
│   ├── prompt.go            # goalVocabulary const + gloss prose → tool.VocabularyLine/PromptGlossBlock
│   └── parse.go             # validGoals → tool.WorldGoals; cap literals → tool Cost lookups
├── sim/
│   ├── plan.go              # planGoals dies
│   ├── loop.go              # door validates via tool.PlanStepGoals + roster (ladder untouched)
│   ├── agents.go            # intentDuration switch → registry-built duration table
│   ├── policy.go            # resolveGoal switch → name-keyed resolver table (semantics identical)
│   └── toolcheck.go         # NEW: startup coverage check (resolver+duration per world tool;
│                            #      expressive Events ⊆ injectSocialWhitelist)
├── metatron/turn.go         # nudge forms/caps via tool.RosterMetatron + Cost
└── daemon/                  # boot calls tool.Validate() + sim coverage check; error aborts
```

**Structure Decision**: leaf-package registry per the `internal/cognition` precedent
(R1); behavior stays in `sim`/`metatron` keyed by tool name (R2). Executor arms
(`executeAtTarget`, `workDuration`), reducer arms, reflex ladder, both door signatures,
and the whitelist are deliberately untouched.

## Complexity Tracking

No constitution violations; table intentionally empty.
