---
id: TASK-64
title: 'Metatron instruction surface: staged charter + skill files + gated tool roster'
status: In Progress
assignee: []
created_date: '2026-07-23 03:27'
updated_date: '2026-07-23 05:20'
labels:
  - review-2026-07-22
  - teaching-game
dependencies: []
priority: high
ordinal: 57000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-22 team review (new-ideas item 2, REFRAMED by the client 2026-07-22): original proposal was player-authored villager personas; the client ruled that out — "indirect influence is the entire point", villagers stay sealed behind the persona firewall. The editable surface is the ANGEL, and it should mirror how real assistant configuration works so the skills transfer to Claude/ChatGPT at work.

Client-stated progression this task builds the substrate for: (1) prompt a base Metatron conversationally like Claude/ChatGPT — he has some basic tools; (2) learn to edit his instructions — the charter becomes a CLAUDE.md-shaped instruction file the player authors; (3) pick additional capabilities your angel can "do" in the world — SKILL.md-shaped skill files plus a gated tool roster unlocked over time.

Scope: evolve the single charter.md into a staged instruction surface. (a) Keep the per-read hot-reload discipline (metatron/charter.go — re-read every turn, zero watcher machinery) for every instruction file. (b) Add a skills/ dir in the world save: SKILL.md-style files the player writes, composed into the turn system prompt beneath the fixed frame (the two non-negotiables stay in the fixed frame — turn.go:388-425 — NEVER in editable text). (c) Make the tool roster per-world configurable: a capability manifest declaring which nudge/miracle/query tools this world grants the angel, so tools are unlockable. (d) While in there, fix review finding: collapse the hand-written prose tool list in turnSystemPrompt (turn.go:396-422) into registry-derived schemas — it duplicates the native declarations AND the reducer miracleCost table (drift surface). (e) Provenance in the TUI status: which instruction files are default vs custom, which tools granted.

This is the feature substrate for the curriculum ladder task (gate-to-feature pathway); the curriculum task depends on this one.

Spec: specs/021-metatron-instruction-surface
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Charter keeps per-read hot-reload; player edits take effect next turn with no restart
- [ ] #2 Skill files in the world dir compose into the turn prompt beneath the fixed frame; fixed-frame invariants provably not overridable from any editable file
- [ ] #3 Per-world capability manifest gates which tools appear in the roster sent to the model; ungranted tools are structurally absent (not declared), not prose-forbidden
- [ ] #4 Prose tool list in turnSystemPrompt replaced by registry-derived schemas; miracle costs have one source of truth
- [ ] #5 TUI metatron status shows instruction-file provenance (default/custom) and granted tool set
- [ ] #6 Spec Kit spec written and linked via spec-bridge before implementation (non-trivial task)
- [ ] #7 Spec phase: Setup
- [ ] #8 Spec phase: Foundational (blocking prerequisites in `internal/tool` + `internal/sim`)
- [ ] #9 Spec phase: User Story 1 — Player-authored skill files (Priority: P1) 🎯 MVP
- [ ] #10 Spec phase: User Story 2 — World-scoped capability grants (Priority: P2)
- [ ] #11 Spec phase: User Story 3 — Provenance + grants in the TUI (Priority: P3)
- [ ] #12 Spec phase: Polish & Cross-Cutting
<!-- AC:END -->



## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Started 2026-07-23. Running full Spec Kit flow (specify → clarify → plan → tasks → spec-bridge link) before delegated implementation, per constitution Principle V. Running in parallel with TASK-63 (separate agent); known small touchpoint: Metatron TUI pane (63 AC#3 vs 64 AC#5).

Spec written: specs/021-metatron-instruction-surface/spec.md (3 user stories, FR-001..012, SC-001..006, quality checklist passing, zero NEEDS CLARIFICATION — ambiguities resolved from TASK-64/68 board artifacts + existing charter/registry semantics per Principle I; chosen defaults recorded in spec Assumptions). NOTE: .specify/feature.json left pointing at specs/020 (TASK-63 agent's live pointer, shared state) — downstream speckit commands for this task get SPECIFY_FEATURE_DIRECTORY=specs/021-metatron-instruction-surface passed explicitly.

Plan complete: plan.md, research.md (R1-R9 decision log), data-model.md, contracts/{instruction-surface,capability-manifest,status}.md, quickstart.md under specs/021-metatron-instruction-surface/. Key decisions: skills/ + capabilities.json at worldDir root beside charter.md; lexicographic composition, 4000 chars/file, 8 files max, charter-style notices; fixed frame compile-time constant appended LAST on every path; manifest missing=full grant (byte-compat), malformed=permissive fallback+notice; three-layer gating (declaration/prose/door); miracle cost single source moves to leaf internal/tool, sim derives (import direction already exists); Status gains Skills/GrantedTools/ManifestDefault, TUI delta confined to console header (TASK-63 collision minimized). Constitution Check PASS pre+post design. Implementation tier: Opus 4.8 — rubric: cross-package (tool/sim/metatron/ipc/tui) + doctrine-adjacent (fixed frame, injection-sound capability gating).
<!-- SECTION:NOTES:END -->
