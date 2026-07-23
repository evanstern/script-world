# Implementation Plan: Villager Prompt Quality

**Branch**: `task-73-villager-prompt-quality` | **Date**: 2026-07-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/027-villager-prompt-quality/spec.md`

## Summary

Rewrite the villager system prompt (`systemPrompt`, `internal/mind/prompt.go:23-38`)
from a name-repeating paragraph into a crafted three-part frame — one identity
statement, the persona block, tight task framing — while preserving doctrine
byte-for-byte in meaning (exactly one acting tool, read-then-act, muse-is-an-action,
no free-text action path) and keeping the prompt a pure, cacheable function of
`(name, personaText)`. Evaluate a fourth part — one worked exemplar of good tool
selection — and ship it only if measured rejection rates don't regress. The whole
change is eval-gated: scripted-stub suite + a live soak (same seed, same game-time
window, local tier) comparing `rejected_malformed` / `rejected_cardinality` rates,
acting-tool distribution, and prompt token counts, with numbers recorded on TASK-73.

## Technical Context

**Language/Version**: Go 1.x (module `github.com/evanstern/promptworld`)

**Primary Dependencies**: standard library only for the change itself
(`fmt`, `strings`); eval leans on the existing daemon CLI (`promptworld new/start/
speed/stop/tail`) and the local LLM tier (Ollama, `cogito:3b` per current provider
registry) — no new dependencies.

**Storage**: the world's durable event log (via `internal/store`) already records
every tool call as a `cog.tool_call` event with a `verdict` field — rejection rates
and tool distribution are computed from it with `promptworld tail` + `jq`; no new
telemetry.

**Testing**: `go test ./...` (deterministic scripted-stub suites in
`internal/toolloop` and `internal/mind`); new unit tests for the prompt frame
(name-once, empty-persona rendering, byte-identical renders, doctrine keywords).

**Target Platform**: darwin/linux dev machines running the promptworld daemon.

**Project Type**: single Go project (daemon + CLI + TUI).

**Performance Goals**: system-prompt token count recorded per variant; no hard
token ceiling, but growth is a conscious recorded choice (local 3B-class models
degrade with prompt length — the eval gate is the arbiter).

**Constraints**: prompt must remain a stable per-agent prefix — it is sent as the
single `system` block with `cache_control: ephemeral`
(`internal/llm/providers.go:571-574`), so it must stay a pure function of
`(name, personaText)` with zero dynamic content; doctrine wording may change,
doctrine meaning may not.

**Scale/Scope**: one function rewritten (+ helper if the identity line is shared),
~4 new/updated tests, one eval script, one spec dir; `internal/mind/meeting.go:89`
(law-phrasing prompt) duplicates the old identity line but is a different surface
(KindMeeting) and stays out of scope.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Evidence |
|-----------|--------|----------|
| I. Artifact-Grounded Action | PASS | Spec dir `specs/027-villager-prompt-quality/`; eval numbers land on TASK-73 (FR-007); exemplar decision recorded with measured reason (FR-004). |
| II. One Task, One PR | PASS | Branch `task-73-villager-prompt-quality` in `.worktrees/task-73`; root stays on main; single PR carries prompt + tests + eval artifacts. |
| III. Gates Over Assertions | PASS | Spec linked to TASK-73 via spec-bridge BEFORE implementation; ship gate is the eval (SC-001), not vibes; bridge gate holds status to artifacts. |
| IV. Grounding Freshness | PASS (obligation noted) | `docs/wiki/agent-mind.md` lists `internal/mind/prompt.go` as a source — wiki re-pin (`/grounding-wiki:wiki-update`) is part of Done for this task. |
| V. Model-Tiered Workflow | PASS | Planning (this spec/plan/tasks) on Fable 5. Implementation delegated to `spec-implementer` at **Opus 4.8**: doctrine-adjacent behavior change in `internal/mind` (rubric: doctrine-adjacent + behavior-affecting eval-gated slice). Tier + justification recorded on TASK-73. |

**Post-Phase-1 re-check**: PASS — design adds no projects, no new dependencies, no
new telemetry; Complexity Tracking stays empty.

## Project Structure

### Documentation (this feature)

```text
specs/027-villager-prompt-quality/
├── spec.md              # Feature spec
├── plan.md              # This file
├── research.md          # Phase 0: decisions (eval method, soak shape, exemplar design, token counting)
├── data-model.md        # Phase 1: prompt frame structure + eval record shape
├── quickstart.md        # Phase 1: how to run the before/after eval end-to-end
├── contracts/
│   └── system-prompt.md # Phase 1: the prompt frame contract (doctrine invariants, cache purity)
├── checklists/
│   └── requirements.md  # Spec quality checklist (done)
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/mind/
├── prompt.go            # systemPrompt rewrite (the change)
└── prompt_test.go       # new frame tests: name-once, empty persona, purity, doctrine

scripts/
└── eval-prompt-73.sh    # soak driver + verdict/distribution tally (jq over `promptworld tail`)

specs/027-villager-prompt-quality/
└── eval/                # recorded eval outputs per variant (old / new / new+exemplar)
```

**Structure Decision**: single-project layout as-is; the only production change is
`internal/mind/prompt.go`. The eval driver lives in `scripts/` (repo has no
scripts/ dir yet — created here) and its recorded outputs in the spec dir's
`eval/`, so the ship-gate evidence is tracked, reviewable, and referenced from
TASK-73.

## Complexity Tracking

No constitution violations — table intentionally empty.
