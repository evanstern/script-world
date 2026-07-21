# Implementation Plan: Conversation Robustness

**Branch**: `011-conversation-robustness` | **Date**: 2026-07-21 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/011-conversation-robustness/spec.md`

## Summary

The conversation scene runner (`internal/mind/convo.go` `runConversation`) discards a
whole scene on any single bad model reply, at two sites: mid-dialogue (one failed
utterance, convo.go:194-199) and at the terminal summary (one malformed outcome reply,
convo.go:204-210). Add bounded tolerance — one retry at the outcome site, one
retry-or-skip at the utterance site — plus raw-reply persistence on parse failure,
outcome-prompt hardening with a lenient-parse fallback for the observed unquoted-gist
shape, and a recorded MLX `reasoning_effort` probe. All changes confined to
`internal/mind` (convo.go, parse.go, telemetry payload) with fake-`Submitter` fault
injection tests; no orchestrator, routing, or event-schema-breaking changes.

## Technical Context

**Language/Version**: Go 1.x (repo toolchain; module `github.com/evanstern/script-world`)

**Primary Dependencies**: stdlib only in the touched package (`encoding/json`, `strings`,
`log`); internal deps `internal/llm` (Submitter seam, Request/Response), `internal/sim`
(payload types, outcome constants), `internal/store` (Event)

**Storage**: append-only `events` table via existing injection paths (`InjectSocial`,
`emitCog`); no schema change — new/extended JSON payload fields only

**Testing**: `go test ./internal/mind/` with the existing fake-`Submitter` seam
(`mind.go:23-26`) and convo_test.go patterns (`TestConversationRunsAndLands`,
`TestConversationFailureInjectsNothing`)

**Target Platform**: the `scriptworld` daemon (darwin/arm64 dev; any Go target)

**Project Type**: single Go module, daemon + CLI

**Performance Goals**: happy path unchanged (zero added calls/latency, FR-009);
worst-case scene adds at most 2 extra LLM calls (one per site), inside the existing
`convoDeadline = 10min` scene bound

**Constraints**: all-or-nothing scene landing preserved (FR-003); retries ride the
existing `KindConversation` prio lane under the same admission/staleness rules (FR-007);
persisted raw replies bounded (truncate with marker) so event payloads stay small

**Scale/Scope**: ~4 functions in 2 files + 1 telemetry payload field + tests;
one manual probe script recorded on board TASK-42

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.* (v1.1.0)

- **I. Artifact-Grounded Action** — PASS: spec + plan + board TASK-42 carry the
  evidence (live-world failure log, loss counts); probe findings land on the task.
- **II. One Task, One PR** — PASS: TASK-42 → branch `task-42-conversation-robustness`
  in `.worktrees/task-42` → one PR. Spec phases are internal breakdown.
- **III. Gates Over Assertions** — PASS: spec-bridge:link before implementation;
  status advances only with artifacts (tests, commits, probe notes).
- **IV. Grounding Freshness** — PASS (planned): `docs/wiki/agent-mind.md` and
  `docs/wiki/social-fabric.md` list convo.go among sources; wiki-update is a
  required post-merge step in tasks.md.
- **V. Model-Tiered Workflow** — PASS: planned on Fable 5; implementation delegated
  to `spec-implementer` pinned to **Opus 4.8** — rubric: `internal/mind`
  orchestration, doctrine-adjacent (all-or-nothing landing semantics), and
  robustness changes where a prior-generation site (TASK-8-era) shipped live
  defects. Recorded on the board task.

**Post-Phase-1 re-check**: PASS — design adds no new packages, no new event types
(extends one existing payload), no orchestrator surface change. No Complexity
Tracking entries needed.

## Project Structure

### Documentation (this feature)

```text
specs/011-conversation-robustness/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── telemetry.md     # cog.outcome payload extension + retry semantics
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/mind/
├── convo.go        # runConversation: utterance tolerance + outcome retry;
│                   # outcome(): hardened prompt
├── parse.go        # parseOutcome: lenient unquoted-gist fallback (shared shape
│                   # with parseSay kept intact)
├── telemetry.go    # cogOutcomeEvent: optional raw-reply field (bounded)
├── convo_test.go   # fault-injection tests via fake Submitter
└── parse_test.go   # lenient-parse table tests (observed malformed shapes)

scripts/ (or scratch, not shipped)
└── probe-mlx-reasoning.sh  # FR-008 manual probe; findings recorded on TASK-42
```

**Structure Decision**: single-package change inside `internal/mind`; the probe is a
non-shipped investigation artifact whose durable output is a board-task note.

## Complexity Tracking

No constitution violations; table intentionally empty.
