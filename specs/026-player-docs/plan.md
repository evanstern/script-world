# Implementation Plan: Player Docs — HTML user documentation + docs-freshness skill

**Branch**: `026-player-docs` | **Date**: 2026-07-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/026-player-docs/spec.md`

## Summary

Ship `docs/player/` — seven self-contained, theme-aware HTML pages (index + six topic
pages) written for players and rendered as a plain-language projection of the grounded
corpus (`docs/wiki/`, `README.md`, `docs/llm-providers.md`) — plus a project skill
`.claude/skills/player-docs/` whose Node script makes freshness machine-checkable: every
page records its sources and their pinned commits in HTML meta tags; the script compares
those pins to the corpus's current pins and reports fresh/stale/missing/broken without
writing; the skill regenerates only stale pages, and regeneration when fresh is a no-op.

## Technical Context

**Language/Version**: HTML/CSS (static pages, no JS required); Node ≥ 18 ESM for the
freshness script (matches the educate `progress.mjs` precedent; Node is already a
project prerequisite per memory/tooling)

**Primary Dependencies**: none — no external assets in pages, no npm packages in the
script (stdlib `fs`/`path`/`child_process` for git only)

**Storage**: files in the repo — `docs/player/*.html`, skill under
`.claude/skills/player-docs/`

**Testing**: the freshness script's `--check` mode is itself the test surface (exit
codes for gating); validation scenarios in quickstart.md exercise fresh/stale/no-op
paths by manipulating recorded pins

**Target Platform**: any modern browser opening files from disk (file://, offline);
script runs on macOS/Linux shells

**Project Type**: documentation + tooling (no Go code changes)

**Performance Goals**: n/a beyond "check completes in seconds on 36 notes / 7 pages"

**Constraints**: pages fully self-contained (no external scripts/styles/fonts/images);
legible under both `prefers-color-scheme` values; check mode writes nothing; regenerate
is byte-identical no-op when nothing is stale

**Scale/Scope**: 7 HTML pages, 1 skill (SKILL.md + 1 script), CLAUDE.md pointer edit

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Artifact-Grounded Action | PASS | Deliverables are files + a board-recorded audit; provenance meta tags make grounding itself an artifact |
| II. One Task, One PR | PASS | TASK-82 → one worktree (`.worktrees/task-82`), one branch, one PR; spec phases are internal breakdown |
| III. Gates Over Assertions | PASS | Freshness is proven by the script's check (exit code), not asserted; skill runs standalone — composes with wiki-update via files, never by call |
| IV. Grounding Freshness | PASS | Pages are a downstream rendering of docs/wiki and record the pins they inherit; no wiki sources are touched by this feature, so no re-pin is triggered |
| V. Model-Tiered Workflow | PASS | Fable 5 planned/spec'd; implementation delegates to spec-implementer on **Sonnet** (doc/rendering + script work — routine tier; no concurrency/architecture) |

**Post-Phase-1 re-check**: PASS — design added no projects, no external deps, no
cross-plugin calls; Complexity Tracking stays empty.

## Project Structure

### Documentation (this feature)

```text
specs/026-player-docs/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── provenance-and-check.md   # meta-tag format + script CLI contract
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
docs/player/
├── index.html                 # nav hub; links all pages; no factual claims
├── getting-started.html       # install → new → start → attach/ui
├── playing-via-metatron.html  # Metatron, the charter, charges, miracles
├── time-and-speed.html        # game clock, speed ladder, pause/resume
├── reading-the-story.html     # chronicle, catch-up, TUI panes
├── the-ai-behind-the-village.html  # minds, local/cloud tiers, plain-language
└── llm-setup-basics.html      # llm.json for non-engineers; defers to operator guide

.claude/skills/player-docs/
├── SKILL.md                   # generation/regeneration procedure + when to run
└── scripts/
    └── check-freshness.mjs    # --check reporter; machine-checkable staleness

CLAUDE.md                      # one-line pointer: run player-docs after wiki-update
```

**Structure Decision**: pages live beside the other doc corpora (`docs/wiki/`,
`docs/course/`) as a third, player-facing projection; the script lives inside the skill
directory (skill-owned tooling, mirroring the educate plugin's `scripts/progress.mjs`
precedent) so the skill is self-contained and removable as a unit.

## Complexity Tracking

No constitution violations — table intentionally empty.
