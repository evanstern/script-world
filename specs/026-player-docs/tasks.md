# Tasks: Player Docs — HTML user documentation + docs-freshness skill

**Input**: Design documents from `/specs/026-player-docs/`
**Prerequisites**: plan.md, research.md, data-model.md, contracts/provenance-and-check.md, quickstart.md
**Tests**: not requested — validation runs through quickstart.md scenarios V1–V6.

## Phase 1: Setup

- [X] T001 Create `docs/player/` directory and verify Node ≥ 18 available (`node --version`); no scaffolding files — pages arrive in later phases

## Phase 2: Foundational (blocks all user stories)

- [X] T002 Author `.claude/skills/player-docs/SKILL.md`: frontmatter (`name: player-docs`, description stating it generates/refreshes docs/player and is the recommended follow-on after `/grounding-wiki:wiki-update`); the check-first regeneration procedure (run script → stop if exit 0 → rewrite only stale/missing pages → re-run script, must exit 0; fresh pages never opened for writing); the canonical page HTML skeleton with the shared inlined CSS block (system font stack, readable measure, CSS custom properties with `prefers-color-scheme: dark` overrides, no external assets, no JS); the provenance meta-tag format per contracts/provenance-and-check.md; and the page→source mapping table from research.md D5

## Phase 3: User Story 1 — new player gets to a running world (P1) 🎯 MVP

**Goal**: index + getting-started render standalone, grounded and provenance-tagged.

**Independent Test**: quickstart V1 on these two pages; every command matches README.md / cli-promptworld / daemon-lifecycle / tui-client at their pinned commits.

- [ ] T003 [US1] Write `docs/player/getting-started.html` using the SKILL.md skeleton: plain-language install→build→`new`→`start`→`attach`/`ui` walkthrough projected from README.md, docs/wiki/cli-promptworld.md, docs/wiki/daemon-lifecycle.md, docs/wiki/tui-client.md; `promptworld-docs:source` meta per source at its current pin (wiki: `verified_against`; README: last-touching commit); link back to index.html
- [ ] T004 [US1] Write `docs/player/index.html`: nav hub linking all six topic pages with one-line player-facing blurbs; `generated-by` meta only (no source tags); same inlined CSS; note it links pages that arrive in Phase 4 (relative links fine before those files exist, but all must resolve by end of Phase 4)

**Checkpoint**: open index.html from disk — index + getting-started render in light and dark, offline.

## Phase 4: User Story 2 — player learns to play (P2)

**Goal**: the four gameplay pages, each a grounded projection of its D5 sources.

**Independent Test**: spot-check claims per page against declared sources at recorded pins; llm page stays at "get it working" depth and links/mentions docs/llm-providers.md for operator detail.

- [ ] T005 [P] [US2] Write `docs/player/playing-via-metatron.html` from docs/wiki/metatron.md, docs/wiki/metatron-miracles.md, docs/wiki/governance.md — Metatron as the player's sole verb, the charter as the one editable prompt, charges/dreams/omens/miracles in plain language; provenance metas at current pins
- [ ] T006 [P] [US2] Write `docs/player/time-and-speed.html` from docs/wiki/game-clock.md, docs/wiki/sim-loop.md, docs/wiki/cli-promptworld.md — tick=1 game second, speed ladder incl. default 4x and max, pause/resume as player verbs, always-on world; provenance metas
- [ ] T007 [P] [US2] Write `docs/player/reading-the-story.html` from docs/wiki/chronicle.md, docs/wiki/tui-client.md, docs/wiki/event-log.md — the chronicle as the catch-up mechanism, TUI panes, tail/attach views, where the story comes from; provenance metas
- [ ] T008 [P] [US2] Write `docs/player/the-ai-behind-the-village.html` from docs/wiki/agent-mind.md, docs/wiki/cognition.md, docs/wiki/llm-orchestrator.md, docs/wiki/nightly-consolidation.md, docs/wiki/social-fabric.md — plain-language: villager minds, local vs cloud tiers, reflexes vs LLM decisions, sleep-time memory digestion, gossip/relationships; no engineering vocabulary unexplained; provenance metas
- [ ] T009 [P] [US2] Write `docs/player/llm-setup-basics.html` from docs/llm-providers.md, docs/wiki/llm-orchestrator.md, README.md — minimum llm.json a non-engineer needs to get inference running; explicitly defer registry reference/migration depth to docs/llm-providers.md by mention; provenance metas
- [ ] T010 [US2] Verify every index.html link resolves and every topic page links back to index; fix any dangling nav

**Checkpoint**: all seven pages render standalone; page set covers FR-002 topics.

## Phase 5: User Story 3 — provable freshness (P3)

**Goal**: the check script + skill make staleness machine-checkable and regeneration idempotent.

**Independent Test**: quickstart V2–V5 (all-fresh clean run, exact staleness, missing/broken-ref, no-op).

- [ ] T011 [US3] Implement `.claude/skills/player-docs/scripts/check-freshness.mjs` per contracts/provenance-and-check.md: Node ≥ 18 ESM, zero deps; resolve repo root via `git rev-parse --show-toplevel`; expected-page-set constant (the seven slugs); line-oriented meta parsing; wiki pin from `verified_against:` frontmatter, non-wiki pin via `git log -1 --format=%H -- <path>`; verdict precedence missing > broken-ref > stale > fresh; text output lines + summary; `--check` alias, `--json` report; exit codes 0/1/2; writes nothing ever
- [ ] T012 [US3] Run quickstart V2 (all fresh, exit 0, `git status` clean), V3 (single-pin tamper → exactly one stale, exit 1, then revert), V4 (missing + broken-ref surfaced) from repo root and from a subdirectory; fix script until all pass
- [ ] T013 [US3] Add the CLAUDE.md pointer: one line in the PDLC/rules area naming `player-docs` (regenerates docs/player from the wiki; run after `/grounding-wiki:wiki-update`; check mode: `node .claude/skills/player-docs/scripts/check-freshness.mjs --check`)

**Checkpoint**: V2–V4 pass; skill procedure + script agree with the contract.

## Phase 6: Polish & Validation

- [ ] T014 Run quickstart V1 end-to-end (all pages, both themes, offline) and V5 (regeneration no-op: check exits 0 → skill stops, `git status --porcelain docs/player` empty)
- [ ] T015 Run quickstart V6 grounding spot-audit: ≥2 claims per topic page verified against a declared source at its recorded pin via `git show <pin>:<path>`; write the audit table (page, claim, source, verdict) into the implementation report for the orchestrator to record on TASK-82

## Dependencies & Execution Order

- Phase 1 → Phase 2 → Phase 3 (US1) → Phase 4 (US2) → Phase 5 (US3) → Phase 6
- US1 before US2 only because index (T004) fronts the set; T005–T009 are mutually parallel [P]
- US3 depends on pages existing (the script's expected set must find them) but not on their prose
- T013 is independent of T011/T012 output but stays in US3 (documents the mechanism)

## Implementation Strategy

MVP = Phase 3 (index + getting-started). Incremental delivery: each phase checkpoint is
independently demonstrable; all phases land as commits on the single TASK-82 branch
(`.worktrees/task-82`), one PR.
