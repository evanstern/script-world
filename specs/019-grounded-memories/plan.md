# Implementation Plan: Grounded Memories — Situated Episodic Capture & Agent-Authored Journal

**Branch**: `019-grounded-memories` | **Date**: 2026-07-22 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/019-grounded-memories/spec.md`

## Summary

Two layers over the agent memory substrate. **Layer 1** enriches every episodic memory at emission with structured, deterministic context — where (coords + place description), why (the driving intent's planner reason, when one exists), and refs (the conversation id for conversation memories) — carried in `MemoryAddedPayload`, reduced onto `Memory`, and rendered by the scribe into soul.md. The one structural enabler: the planner's reason currently dies at injection time (`InjectArgs.Reason` → one `agent.thought`, then dropped), so `Intent` gains a `Reason` field the reducer populates from the already-emitted `agent.intent_set` event, making the reason available at completion time when the executor bakes memories. **Layer 2** gives each agent a self-authored markdown journal: world state (`Agent.Journal`), mutated only through two new whitelisted event types (`journal.entry_written` / `journal.entry_deleted`) landed by two new Expressive registry tools, read through two new Read registry tools (the first production Read tools), budget-enforced (4,000 runes) in the reducer's dry-run so the gate — not the handler — decides, and rendered by the scribe as `agents/<name>/journal.md`. Everything is baked at emission and reducer-applied: replay reproduces byte-identical souls and journals with zero model calls.

## Technical Context

**Language/Version**: Go 1.26 (module `github.com/evanstern/promptworld`)

**Primary Dependencies**: stdlib-heavy daemon; internal packages only for this feature — `internal/sim` (state, reducer, executor, doors), `internal/mind` (planner driver, convo, handlers), `internal/tool` (registry/rosters), `internal/toolloop` (bounded loop, TASK-52), `internal/scribe` (rendered views), `internal/llm` (config), `internal/persona` (per-agent file paths)

**Storage**: append-only SQLite event log (`internal/store`) — source of truth; `agents/<name>/*.md` are regenerable views

**Testing**: `go test ./...` — table-driven unit tests + the existing replay/determinism suite; sim boot gates (`tool.Validate`, `sim.ValidateToolCoverage`) double as unit tests

**Target Platform**: macOS/Linux daemon (always-on world process, attachable clients)

**Project Type**: single Go module, event-sourced simulation daemon

**Performance Goals**: no new model calls (Layer 1 is fully deterministic; Layer 2 adds tools to the existing planner loop budget — no new cognition channels); journal search is in-memory over ≤4,000 runes per agent — negligible

**Constraints**: byte-deterministic replay (live Apply == replay Apply, no model calls, no out-of-band lookups); mind-authored events pass only through the `injectSocialWhitelist` door; pre-019 event logs must replay and render exactly as before (payload fields all `omitempty`)

**Scale/Scope**: 8 agents/world; ~4,000-rune journal per agent; touches 6 internal packages, no new external dependencies

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Check | Status |
|-----------|-------|--------|
| I. Artifact-Grounded Action | Spec/plan/research/data-model/contracts live in `specs/019-grounded-memories/`; board task TASK-16 linked via spec-bridge before implementation; grounding facts pinned to file:line in research.md | PASS |
| II. One Task, One PR | TASK-16 → one branch (`task-16-grounded-memories` in `.worktrees/task-16`) → one PR; spec phases are internal breakdown | PASS |
| III. Gates Over Assertions | Journal budget enforced in the reducer (the gate), not handler courtesy; boot coverage gates extended to the new tools/events; bridge gate governs board status | PASS |
| IV. Grounding Freshness | Plan grounded on wiki notes verified at 6444c29 (current main) + a fresh file:line sweep (research.md); wiki-update scheduled post-merge for touched sources | PASS |
| V. Model-Tiered Workflow | Fable 5 planned/gates; implementation delegated to `spec-implementer`. Tier per rubric: cross-package + doctrine-adjacent (whitelist, doors, reducer) → **Opus 4.8** for the core slices; routine rendering/tests may run Sonnet. Recorded on TASK-16 at delegation time | PASS |

**Post-design re-check (after Phase 1)**: no violations introduced — no new packages, no new dependencies, no bypass of existing doors; the journal door is the existing `InjectSocial` whitelist + reducer dry-run. Complexity Tracking stays empty. PASS.

## Project Structure

### Documentation (this feature)

```text
specs/019-grounded-memories/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── memory-context.md    # MemoryAddedPayload v2 + rendering contract
│   └── journal-tools.md     # journal tool + event contracts
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/
├── sim/
│   ├── agents.go        # MemoryAddedPayload + Memory + Intent gain context fields
│   ├── memory.go        # constructors gain situated variants; journal budget const
│   ├── executor.go      # situated template call sites (where/why baked at emission)
│   ├── state.go         # Apply arms: memory context copy; intent reason; journal events
│   ├── loop.go          # injectSocialWhitelist + journal event admission
│   ├── social.go        # (read-only reference: ConversationTurnPayload.Conv)
│   ├── journal.go       # NEW: Journal state type, budget enforcement, JournalEntry
│   └── toolcheck.go     # coverage gate picks up new Expressive tools' Events
├── mind/
│   ├── convo.go         # gist memory gains Conv ref
│   ├── handlers.go      # journal tool handlers (write/delete via InjectSocial; search/read from replica)
│   └── mind.go          # villagerHandlers registration
├── tool/
│   ├── registry.go      # 4 new tools: write_journal_entry, delete_from_journal (Expressive), search_journal, read_journal (Read)
│   └── roster.go        # LoopRosterVillager gains the journal tools
├── toolloop/            # unchanged (Read-tool path already specified by 017)
├── scribe/
│   └── scribe.go        # soul.md situated memory lines; NEW journal.md render
├── llm/                 # unchanged (no new call kinds)
└── persona/
    └── files.go         # JournalPath helper

Tests alongside: internal/sim/*_test.go (reducer, budget, replay), internal/mind/*_test.go (handlers), internal/tool/*_test.go (registry/roster), internal/scribe/*_test.go (renders), plus the existing replay/determinism suite extended with journal + situated-memory fixtures.
```

**Structure Decision**: single Go module, existing package boundaries only. The one new file is `internal/sim/journal.go` (journal state + budget rule live with the reducer that enforces them). No new packages: the journal is world state (sim), its tools are registry entries (tool), its handlers are mind-side (mind), its view is a scribe render (scribe) — exactly where each concern already lives.

## Complexity Tracking

> No Constitution Check violations — table intentionally empty.
