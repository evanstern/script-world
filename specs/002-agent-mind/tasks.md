# Tasks: Agent Mind v1

**Input**: Design documents from `/specs/002-agent-mind/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Included — determinism, the firewall, and the window bound are spec
success criteria only provable by tests.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

- [X] T001 Extend sim types in internal/sim/agents.go — Memory struct, Agent.Memories/IdleSince/NearDeath, salience + grace + cadence constants, MemoryAddedPayload/ThoughtPayload; agent count 4→8 with names for the new four

---

## Phase 2: Foundational (blocking)

- [X] T002 Reducer support in internal/sim/state.go — apply agent.memory_added, agent.thought (no-op), intent_set source field, IdleSince stamping on intent-clearing/wake/genesis, NearDeath latch
- [X] T003 Memory emission in internal/sim/executor.go + internal/sim/memory.go — deterministic heuristics per research R2 (talk, builds, hunt, starving-forage, near-death, death witnessed radius 8, cold night survived), templated text
- [X] T004 Working-memory window in internal/sim/memory.go — SelectMemories (salience × recency half-life 24 game-h, top K−2 + 2 seeded tail picks bucketed to cadence, reverse-chron presentation)
- [X] T005 Reflex demotion in internal/sim/executor.go — reflex fires only when tick−IdleSince ≥ 120
- [X] T006 inject_intent loop command in internal/sim/loop.go + internal/sim/policy.go — Loop.InjectIntent(agent, goal, targetAgent, reason); validation (alive, awake), deterministic target resolution via shared resolveGoal (incl. talk_to → target tile), emits intent_set(source:planner) + agent.thought
- [X] T007 [P] Sim unit tests in internal/sim/memory_test.go + sim_test.go updates — window bound/determinism/serendipity stability (AC#3), memory emission cases, IdleSince/grace behavior, determinism + replay harnesses re-proven with injected commands in the timeline, 8-agent survival

**Checkpoint**: `go test ./internal/sim/` green.

---

## Phase 3: US2 — personas & souls (files first: US1's prompts need them)

- [X] T008 [US2] internal/persona — 8 authored personas (constants), Genesis(dir) writing agents/<name>/{persona.md,soul.md} with persona mode 0444; Load(dir) map for the mind
- [X] T009 [US2] internal/scribe — always-on soul.md writer: replica from notify stream, regenerate on memory/death events per contracts/agent-files.md
- [X] T010 [US2] Wire genesis + scribe — cmd/promptworld `new` calls persona.Genesis; daemon fans notify out to server + scribe (always)
- [X] T011 [P] [US2] Tests in internal/persona/persona_test.go + internal/scribe/scribe_test.go — 0444 + content stability, soul render format, regeneration from replayed events

**Checkpoint**: new worlds have villagers with natures and growing souls, LLM or not.

---

## Phase 4: US1 + US3 — the mind driver

- [X] T012 [US1] internal/mind/prompt.go — system prefix (persona + instruction block) and user suffix (status, nearby summary, K-line window via sim.SelectMemories) per contracts/planner-prompt.md
- [X] T013 [US1] internal/mind/parse.go — first-JSON-object extraction, goal vocabulary validation, target name resolution
- [X] T014 [US1] internal/mind/mind.go — replica + scheduler: 1800-tick stagger cadence, triggers (woke, idle-completion, night_started, encounter w/ 2-game-h pair cooldown), orchestrator Submit(kind planner), InjectIntent on success, skip-and-log on any failure
- [X] T015 [US1] Daemon wiring — start mind when orchestrator exists; fan-out notify to server + scribe + mind
- [X] T016 [P] [US1] Mind integration tests in internal/mind/mind_test.go — mock local model: cadence fires per agent, wake/night triggers fire, planner intents reach the log with source:planner and executor acts, garbage output → no event + reflex floor holds, dead endpoint → reflex world (SC-004)
- [X] T017 [US3] Prompt-window assertion in internal/mind tests — agent with 100+ memories → prompt contains ≤ K memory lines (AC#3 end-to-end)

**Checkpoint**: mock-model world shows planner-driven villagers; degraded world shows reflex floor.

---

## Phase 5: Polish

- [X] T018 [P] Full -race suite + e2e re-run; live smoke against real Ollama (Scenario A/B/C from quickstart.md); record results in specs/002-agent-mind/quickstart-results.md
- [X] T019 [P] TUI souls pane: show each agent's newest memory line under the needs gauges; README status update
- [X] T020 Wiki update (agent-mind note; re-pin affected notes), board sync

---

## Dependencies

Setup → Foundational → {US2 files, US1 mind}; US1 needs T008 (personas for prompts);
Polish last. One TASK, one PR on `task-7-agent-mind`.
