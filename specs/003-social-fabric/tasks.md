# Tasks: Social Fabric

**Input**: Design documents from `/specs/003-social-fabric/`
**Tests**: Included — every AC is only provable by tests.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Foundational sim core (blocking)

- [ ] T001 Types + state in internal/sim/social.go + agents.go + state.go — Relation/Debt/Rumor/KnownRumor, State.Relations/Debts/Rumors/NextDebtID/NextRumorID, Agent.Known/LastGive, payload structs
- [ ] T002 Reducer cases (social.go, called from state.go Apply) — relation_changed (lazy+clamp), gave (transfer + settle-or-incur), debt_settled/promise_broken, rumor_told (birth on id 0, variant add, affection-to-subject), secret_seeded, conversation events no-op
- [ ] T003 Reputation + Tellable helpers in social.go — computed reputation; TellableFor(state, teller, listener) picking best memory-about-others or Known rumor (excluding subject==listener, conf>floor)
- [ ] T004 Executor acts in executor.go — give/repay in the encounter slot (starving neighbor, food≥2, 1h cooldown; priority over talk), hourly due-check breaking overdue debts, talk affection deltas, verbatim rumor fallback told during primitive talks (model-free spread)
- [ ] T005 inject_social loop command in loop.go — whitelisted batch, tick re-stamp, atomic apply+append+notify
- [ ] T006 [P] Sim tests in internal/sim/social_test.go — edge rules table (US1), ledger lifecycle + reputation (US2/SC-002), 3-hop provenance + decay + floor (US3/SC-003), secret gate data, determinism/replay with social timelines (SC-005)

**Checkpoint**: model-free social fabric complete and proven.

---

## Phase 2: Secrets + genesis (US3)

- [ ] T007 [US3] persona.Secrets (8 authored) + SecretEvents(); `new` appends tick-0 secret_seeded events; persona tests

---

## Phase 3: Conversations (US4)

- [ ] T008 [US4] internal/mind/convo.go — trigger on agent.talked, slot=1, immutable snapshot ctx, ≤3 utterances/side (KindConversation), outcome call, all-or-nothing inject_social batch per contracts
- [ ] T009 [US4] Prompt + parse additions — utterance/outcome prompts, {"say"} and outcome JSON parsing with clamps
- [ ] T010 [US4] Planner prompt social context (prompt.go): bonds, debts, reputation, top rumor; scribe Bonds section in soul.md
- [ ] T011 [P] [US4] Convo tests in internal/mind/convo_test.go — scripted mock: cap respected, dual gist memories, tone edges, paraphrased rumor_told; failure → nothing injected; slot serialization

---

## Phase 4: Polish

- [ ] T012 [P] Full -race suite; live Ollama smoke (gemma4:12b-mlx) per quickstart; record specs/003-social-fabric/quickstart-results.md
- [ ] T013 Wiki (social-fabric note + re-pins), board sync, PR

## Dependencies
P1 → {P2, P3} → P4. One TASK, one PR on `task-8-social-fabric`.
