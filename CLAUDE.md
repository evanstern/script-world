<!-- pdlc:grounding BEGIN v0.8.0 — planted by pdlc:bootstrap; refreshed wholesale on update. Keep project-specific edits OUTSIDE this block. -->
# script-world — praxis development lifecycle (PDLC)

This project is developed with the **praxisflux** plugin suite. This block is the always-on
grounding: it names the loop, each plugin's role, and the rules that hold between them. The
procedures live in the plugins' skills (lazy-loaded); this block makes the rules apply even
when no skill has triggered.

## The loop

Ground the codebase → plan as specs → build → re-ground → teach/render:

```
grounding-wiki (docs/wiki) ──corpus──▶ codebase-to-course (docs/course)
        │
        └─grounding─▶ spec/plan ──▶ build ──▶ wiki-update (re-ground) ──▶ …
```

## Plugin roles (entry skills)

- **grounding-wiki** — the code-grounded corpus in `docs/wiki/`: per-concept notes pinned to
  the commit they were verified against. Build once with `/grounding-wiki:wiki-build`; after
  merging changes that touch files any note lists as sources, run `/grounding-wiki:wiki-update`.
- **codebase-to-course** — interactive single-page HTML course in `docs/course/`, for
  non-technical readers. Reads `docs/wiki/` as its primary input when present.
- **build** — implements a SPEC handed off through `.handoff/` (`/build:implement`) and
  returns findings to the producer.
- **research** — drop-anywhere cited-fact vaults (`research:research-vault` → `analyze-vault`
  → `vault-artifact`) for grounding external topics.
- **spec-bridge** — the kanban view over Spec Kit specs (see the Spec Kit block below, if
  opted in).

## Rules that always hold

- **Artifact-grounded action:** never do anything without leaving a durable paper trail
  and/or gating against real physical evidence in the project — a file, a git commit, a
  task/issue. Artifacts that survive for human review are the only currency of state and
  decision: a choice living only in a chat turn, or a commitment left as prose where its
  durable home is the tracker, did not happen. Decisions are derived FROM artifacts and
  produce NEW artifacts; a question an existing artifact or principle already answers is
  resolved from it, not re-asked as a preference.
- **One TASK, one PR:** a TASK is a top-level deliverable and maps 1:1 to a pull request —
  one task, one branch, one PR. A SUBTASK (whatever the task system calls it) is internal
  work breakdown and never gets its own PR: subtasks land as commits on the parent TASK's
  single branch and merge together in that TASK's one PR.
- **Gates:** a status can never exceed the artifacts that prove it. Plugins ship Stop hooks
  that enforce this; when a gate blocks, produce the missing artifact — don't argue with the
  gate or edit derived state by hand.
- **Handoffs:** plugins compose only through files + gates, never by calling each other.
  Payloads ride the gitignored `.handoff/` transport; evidence lives in tracked state.
- **Grounding freshness:** `docs/wiki/` is load-bearing, not decoration. Changes that touch
  pinned sources aren't done until the wiki is re-pinned (`/grounding-wiki:wiki-update`).

<!-- pdlc:peer:backlog BEGIN -->
## Backlog.md — the board (officially supported peer)

Backlog.md is this project's kanban; the board is the plan of record. Statuses flow
**To Do → In Progress → Done**.

- Start from `backlog task list --plain`; read a task with `backlog task view TASK-x --plain`.
- Record plans (`--plan`), progress (`--append-notes`), and tick acceptance criteria
  (`--check-ac <n>`) as they come true; finish with `--final-summary` and `-s Done`.
- **One task, one PR:** a top-level TASK gets one branch and one PR. Dotted-id subtasks
  (TASK-x.y) are internal breakdown — they ride the parent task's branch and merge in its
  PR, never their own.
- **Never hand-edit** files under `backlog/` — always the `backlog` CLI, so metadata and
  relationships stay consistent.
<!-- pdlc:peer:backlog END -->

<!-- pdlc:peer:spec-kit BEGIN -->
## Spec Kit — specs drive the work (officially supported peer)

Features are specified with GitHub Spec Kit (`specify`) under `specs/NNN-<feature>/`
(spec.md, plan.md, tasks.md). The spec dir is the source of truth for its feature.

- Put a spec on the board with `spec-bridge:link`; after working a spec, run
  `spec-bridge:sync` to move the linked task, re-mirror phase criteria, and record progress.
- The bridge gate blocks a linked task's status from exceeding what the spec artifacts
  prove — produce the artifact, then sync.
- A spec's linked task is the deliverable: it lands as **one PR**. Spec phases and their
  mirrored criteria are internal breakdown, not PR boundaries.
<!-- pdlc:peer:spec-kit END -->
<!-- pdlc:grounding END -->

## Model-tiered workflow (constitution Principle V)

Planning and implementation run on different model tiers, enforced by delegation
(`.specify/memory/constitution.md`, Principle V):

- **Fable 5 plans:** specs (`speckit-specify`), plans (`speckit-plan`), task generation
  (`speckit-tasks`), clarify/analyze, and board/task creation stay on the main session's
  planning model.
- **Sonnet/Opus implements:** when executing implementation tasks (`speckit-implement`,
  `build:implement`, or any tasks.md/SPEC execution), delegate the code-writing to the
  `spec-implementer` agent (`.claude/agents/spec-implementer.md`, pinned to Sonnet;
  override to Opus via the Agent tool's `model` param for high-complexity specs) —
  never implement inline on the planning model. The planning model orchestrates,
  reviews the agent's findings, and gates.

## Git worktrees — root stays on main

The root checkout (`~/evan/script-world`) is **pinned to `main`** — never check out a
task branch there. All branch work happens in sibling worktrees.

- **Create:** when starting a TASK branch, make a worktree instead of switching:
  `git worktree add ../script-world-task-<N> -b task-<N>-<slug> origin/main`
  (sibling dir named `script-world-task-<N>`, matching the task id). Do the work,
  commit, and open the PR from inside that worktree.
- **Root freshness:** keep the root current with `git fetch origin && git pull --ff-only`
  — at session start and always before cutting a new worktree, so branches fork from
  fresh `origin/main`.
- **Cleanup:** after a TASK's PR merges, `git worktree remove ../script-world-task-<N>`,
  delete the branch (`git branch -d …`), and ff-pull the root.
- One TASK, one worktree — this is the same "one task, one branch, one PR" rule; the
  worktree is just where that branch lives.

## educate — Socratic learning layer (planted by educate:start, adapted for PDLC)

This project also hosts **educate** lessons (Socratic grounding/Q&A sessions) under
`topics/<topic-slug>/<NNN>-<lesson-slug>/`. Files inside a lesson use bare names:
`checklist.md`, `raw-notes.md`, and (when produced) `deck.html`, `guide.md`.

- **Lifecycle (exact words):** `scaffolded` → `taught` → `spec'd` → `built` → `decked` → `done`.
  Scaffold by copying `topics/.template/` at the start.
- **Note-taking cadence (enforced):** `raw-notes.md` is maintained live — one Session-log
  entry per question→answer exchange, written before the next question. A turn with no
  note is an incomplete turn.
- **Run lessons via the `educate:lesson` skill;** delegated builds go through the
  `.handoff/` transport to `build:implement` (see the PDLC block above — same rules).
- **Gate:** `topics/<topic>/progress.json` is the machine source of truth; sync/check with
  `node <educate-plugin>/scripts/progress.mjs --root <root> <topic> --sync|--check`.
  Never advance a lesson's status past the artifacts on disk.
- Decks are single self-contained HTML files built FROM `topics/.template/deck.html`.
