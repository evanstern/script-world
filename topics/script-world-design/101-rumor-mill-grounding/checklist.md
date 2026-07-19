# Understanding Checklist: Socratic grounding — the Rumor Mill hybrid (decision-1)

> Check items off only when the assumption has been **examined and resolved** — confirmed,
> revised, or rejected, with the outcome written down in raw-notes.md. Acknowledgment is not
> resolution.
>
> Inputs: `README.md`, `backlog/decisions/decision-1`, the "Chosen direction" section of
> `research/project-plans.html`, TASK-1 on the board.
> Outcomes recorded in `raw-notes.md` (bracketed refs) and `docs/design/grounded-assumptions.md`.

## 1. The agent mind — what "prompt-programmed" means mechanically
- [x] Two-layer brain (LLM planner / deterministic executor): confirmed [3][11]
- [x] Planner cadence: 30-game-min baseline thought + scene-change triggers + agent↔agent conversations capped at ~5 turns each way [3]
- [x] Nightly consolidation: one cloud call per agent per night; persona/temperament firewall enforced structurally + by automated validator [11]
- [x] Soul drift risk: persona.md outside every write path; validator on consolidator output; drift probes an MVP observation goal [11]

## 2. The nudge — the only write channel
- [x] Nudge economy: 1 charge per 6 game hours, max 3 banked; purpose = reining the operator in [4]
- [x] Nudge forms for MVP: Dream + Omen (two forms so the reach-vs-deniability choice exists) [4]
- [x] Attribution loop: emergent culture in scope as observation, not built system; all player text mediated by Metatron [4][5][6]

## 3. The miscast valve
- [x] Valve = social pressure + observation ("see where the agents get"); death by neglect as valve of last resort; generational turnover/aging parked [7][8]

## 4. Cost & latency
- [x] Ceiling $100/month; local-first (Ollama on always-available MacBook, 9router to other sources); world daemon on homelab [3][10][11]
- [x] Tiers: local = planner/conversations; cloud = consolidation, narrator, dramatic moments (router rule = open question, likely Metatron's job) [3][4]
- [x] Latency hides inside live sim pacing; local throughput sets the real max-speed cap; degraded mode designed as resilience [2][3][10][11]

## 5. Simulation model
- [x] Time: default 1 game-min = 15 real-sec (day ≈ 6 real hours); speed real-time↔max; pause required; always-on daemon [1][2]
- [x] Perception: top-K memory window, reverse-chron + cheap rerank + serendipity mix; whole-soul-in-context rejected; vector RAG parked v2 [11]
- [x] Social systems ALL in v1: relationship graph, rumor objects, promises/debts, needs, economy, norms/votes, secrets [8]

## 6. Terminal UI
- [x] Client/daemon split; Go + Bubble Tea recommended and accepted-pending-veto [9→10 answer, 12]
- [x] Panes: map (default) / chronicle / Metatron console / soul reader [12]

## 7. Procedural generation
- [x] One village area: wood, water, forage, animals; NO starting buildings — Minecraft-style cold-start bootstrap; big-map support an engine requirement, not a v1 feature [12]

## 8. The core loop
- [x] Ambient check-in loop: world runs 24/7; player reads chronicle/souls, converses with Metatron, spends nudges; absence is meaningful [2][4][5]
- [x] MVP slice confirmed (amended): 8 agents, persona/soul + consolidation, dream + omen via Metatron, all social systems, death by neglect, the gru, chronicle, 30 game days [12]

## 9. Stack
- [x] Go daemon + Bubble Tea TUI (recommendation accepted in Q&A flow); SQLite event log + per-run flat files (persona.md/soul.md literal); Ollama + 9router + cloud tier [9][10]

## 10. Outputs
- [x] Grounded-assumptions record: `docs/design/grounded-assumptions.md`
- [x] First-pass task list captured on the Backlog.md board (TASK-2…)
- [x] Candidate Spec Kit specs identified (see grounded-assumptions §Candidate specs)
