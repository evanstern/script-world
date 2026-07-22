<!--
Sync Impact Report
- Version change: 1.0.1 → 1.1.0 (MINOR: Principle V materially expanded to a three-tier
  model scheme; spec-rigor rule added to Development Workflow)
- Modified principles:
  - V. Model-Tiered Workflow — two-tier (plan/implement) split expanded to three tiers
    (Fable 5 planning / Opus 4.8 senior implementation / Sonnet implementation) with an
    escalation rubric; rubric operationalized in .claude/agents/spec-implementer.md
- Added sections:
  - Development Workflow — spec-rigor rule: non-trivial TASKs MUST run full Spec Kit
    before implementation; explicit triviality exemption criteria
- Removed sections: none
- Templates:
  - ✅ .specify/templates/plan-template.md — Constitution Check gate is generic; unaffected
  - ✅ .specify/templates/spec-template.md — no constitution references; unaffected
  - ✅ .specify/templates/tasks-template.md — no constitution references; unaffected
  - ✅ .specify/templates/checklist-template.md — no constitution references; unaffected
  - ✅ CLAUDE.md — Model-tiered workflow section rewritten to match (three tiers + spec rigor)
  - ✅ .claude/agents/spec-implementer.md — escalation rubric added
- Follow-up TODOs: none
-->

# promptworld Constitution

## Core Principles

### I. Artifact-Grounded Action

Nothing happens without a durable paper trail: a file, a git commit, a task on the board,
or an issue. Decisions MUST be derived from existing artifacts and MUST produce new ones.
A choice living only in a chat turn, or a commitment left as prose where its durable home
is the tracker, did not happen. A question an existing artifact or principle already
answers MUST be resolved from it, not re-asked as a preference.

**Rationale:** artifacts that survive for human review are the only currency of state and
decision; chat context evaporates, files and commits do not.

### II. One Task, One PR

A top-level TASK is a deliverable and maps 1:1 to a branch and a pull request. Subtasks
(dotted ids, spec phases, mirrored criteria) are internal breakdown: they land as commits
on the parent TASK's single branch and merge in that TASK's one PR — never their own.
Branch work happens in worktrees under the repo-local, gitignored `.worktrees/` folder;
the root checkout stays pinned to `main`.

**Rationale:** a 1:1 task↔PR mapping keeps the board, git history, and review surface in
lockstep, so any one of them can be audited from the others.

### III. Gates Over Assertions

A status MUST never exceed the artifacts that prove it. When a gate blocks, the remedy is
to produce the missing artifact — never to argue with the gate or hand-edit derived state.
Plugins compose only through files and gates (payloads on the gitignored `.handoff/`
transport; evidence in tracked state), never by calling each other.

**Rationale:** self-reported progress drifts; gates anchored to physical evidence cannot.

### IV. Grounding Freshness

`docs/wiki/` is load-bearing, not decoration. A change that touches files any wiki note
lists as sources is not done until the wiki is re-verified and re-pinned
(`/grounding-wiki:wiki-update`). Downstream renderings (`docs/course/`) read the wiki as
their primary input and inherit its freshness.

**Rationale:** stale grounding is worse than none — it lends false confidence to plans
and specs built on it.

### V. Model-Tiered Workflow

Work runs on three model tiers, and the split is enforced by delegation, not discipline:

- **Planning tier — Claude Fable 5** (Mythos-class): writing specs (`speckit-specify`),
  clarification (`speckit-clarify`), plans (`speckit-plan`), task generation
  (`speckit-tasks`), analysis, board/task creation, gating, and review of implementer
  reports. The planning tier NEVER writes implementation code inline.
- **Senior implementation tier — Claude Opus 4.8**: implements high-complexity slices —
  cross-package or architectural changes; concurrency, scheduling, or governor logic
  (`internal/llm`, `internal/cognition`, `internal/mind` orchestration); doctrine-adjacent
  behavior changes; and any slice whose prior Sonnet attempt failed gates or shipped live
  defects. Also runs adversarial verification passes when the orchestrator requests them.
- **Implementation tier — Claude Sonnet** (default): implements routine and mechanical
  slices — single-package features, view/rendering code, tests alongside code, doc
  reconciliation.

Implementation MUST execute in subagents pinned to the implementing model — the
`.claude/agents/spec-implementer.md` agent definition, which carries the escalation
rubric — never inline on the planning model. Tier escalation is one-way (Sonnet → Opus,
via the Agent tool's `model` parameter); the orchestrator records the tier choice and its
rubric justification on the board task.

**Rationale:** the highest-capability tier is spent where judgment concentrates (specs,
plans, decomposition, gating); execution is matched to slice complexity so quality-risk
concentrates on the senior tier and cost concentrates on the routine tier. Pinning models
in the agent definition makes the split mechanical rather than aspirational.

## Additional Constraints

- Backlog.md is the kanban and the plan of record; statuses flow To Do → In Progress →
  Done. Files under `backlog/` MUST only be modified via the `backlog` CLI.
- Spec Kit directories under `specs/NNN-<feature>/` are the source of truth for their
  feature; the board mirrors them through spec-bridge, and the bridge gate blocks a
  linked task's status from exceeding what the spec artifacts prove.
- The `.handoff/` transport is gitignored payload space; durable evidence never lives
  there.
- educate lessons under `topics/` follow their own lifecycle gate
  (`progress.json`); lesson status MUST NOT advance past the artifacts on disk.

## Development Workflow

The praxis development lifecycle (PDLC) is the loop: ground the codebase
(`docs/wiki/`) → plan as specs (`specs/NNN-*`, linked to the board) → build (one task,
one worktree, one PR) → re-ground (`wiki-update`) → teach/render (`docs/course/`).
Plans MUST pass the plan template's Constitution Check gate before Phase 0 research and
re-check it after Phase 1 design; violations require an explicit Complexity Tracking
entry justifying why no simpler alternative suffices.

**Spec rigor:** every non-trivial TASK MUST go through full Spec Kit — `speckit-specify`
→ `speckit-clarify` (where ambiguity exists) → `speckit-plan` → `speckit-tasks` →
implementation — with the spec directory linked to the board via `spec-bridge:link`
BEFORE implementation starts. A TASK qualifies as trivial ONLY when all of the following
hold: the fix is surgical (single mechanism, narrow blast radius), the diagnosis is
complete and pinned to evidence (file:line root cause recorded on the task), and
acceptance criteria live on the board task. Trivial TASKs still follow Principles I–III
and V (adopted 2026-07-21; precedent: TASK-44).

**Rationale:** implementation launched from an under-specified task fails at a much
higher rate than implementation launched from a spec that survived clarify/analyze —
observed directly in this project when work jumped straight from board notes to code.

## Governance

This constitution supersedes ad-hoc practice for the areas it covers. Amendments are made
via `speckit-constitution` (never hand-edited piecemeal), MUST update the Sync Impact
Report, and version according to semantic versioning: MAJOR for incompatible principle
removals or redefinitions, MINOR for new or materially expanded principles/sections,
PATCH for clarifications. Every plan's Constitution Check MUST verify compliance against
the version named in its footer; runtime development guidance lives in `CLAUDE.md` and
MUST stay consistent with this document.

**Version**: 1.1.0 | **Ratified**: 2026-07-20 | **Last Amended**: 2026-07-21
