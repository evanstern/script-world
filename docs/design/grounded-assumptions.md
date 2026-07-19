# Grounded assumptions — script-world v1

**Source:** Socratic grounding session, 2026-07-18 (TASK-1; lesson
`topics/script-world-design/101-rumor-mill-grounding/`, live log in its `raw-notes.md`).
**Basis:** README.md + decision-1 (Rumor Mill hybrid) interrogated question by question.
Each assumption below is marked **confirmed** / **revised** / **rejected** / **new** relative
to what the README and decision-1 claimed. Bracketed numbers cite raw-notes exchanges.

## The one-sentence v1

> An always-on village of **8 agents** with fixed personas and growing souls, all seven
> social systems live, death by neglect, a gru at night, and one player verb — nudging
> (dream + omen) through an editable **Metatron** — produces, within 30 game days, a legible
> story you feel you authored. [12]

## Time & posture

| Assumption | Outcome |
|---|---|
| "Watch the world run" (README) | **Confirmed — and radicalized.** Live-watched, not day-batched. [1] |
| Session shape | **Revised.** Not a session game: default 1 game-min = 15 real-sec → a game day ≈ 6 real hours; 30+ days = weeks of wall-clock. script-world is an **ambient persistent world** (Tamagotchi × Dwarf Fortress); the chronicle is the catch-up mechanism. [1][2] |
| Always-on | **New.** The sim runs 24/7 with no client attached. Sim = daemon (homelab); terminal UI = attachable client. Closing the client is not pause; **pause is a player verb** (required), speeds adjustable real-time ↔ as-fast-as-affordable. [2] |

## Cost & inference

| Assumption | Outcome |
|---|---|
| "Tiered models + caching are requirements" (decision-1) | **Confirmed with numbers.** Hard ceiling **$100/month**. Local tier (Ollama on the always-available MacBook, 9router to other sources) = planner thoughts + conversations (~3,800+ calls/day — only viable self-hosted). Cloud tier = nightly consolidation (~cheap), chronicle narrator, dramatic moments. [3][10][11] |
| Planner cadence | **New.** Baseline thought every **30 game minutes** per agent + scene-change triggers (wake, encounter, interruption, sleep) + agent↔agent conversations capped at **~5 turns each way**. Local-model throughput — not CPU — sets the real max-speed cap. [3] |
| Drama router | **Open.** What escalates a moment to the cloud tier needs a concrete rule; Metatron (which already watches everything) is the natural owner. [3][4] |
| Inference availability | **Resolved as resilience.** MacBook never sleeps/leaves, but graceful degradation (executor keeps ticking, thoughts queue/reflex, auto-slow) is still designed in. [10][11] |

## The agent mind

| Assumption | Outcome |
|---|---|
| Two-layer brain (LLM planner / deterministic executor) | **Confirmed.** [3][11] |
| Persona/soul split (decision-1) | **Confirmed as THE core data model** — villagers *and* Metatron share it. `persona.md` immutable; `soul.md` sim-written, player-readable. Flat files are load-bearing: literal markdown per agent, per-run. [6][10] |
| Perception / context | **Revised.** Whole-soul-in-context rejected. Working memory = **top-K list, reverse-chronological, cheaply reranked each turn, with a few bottom-of-list memories mixed in** against stagnation. Per-agent reranking-as-emergent-personality: parked, promising. Vector/RAG memory (embed-what-you-hear → search → rerank): **parked for v2** (tool-calling era). [11] |
| "Souls change, natures don't" firewall | **Mechanized.** (a) Structural: persona.md outside every write path; (b) automated validator on consolidation output rejects temperament drift; (c) drift observation is an explicit MVP question. [11] |
| Nightly consolidation | **Confirmed.** One cloud call per agent per game night (≈ every 6 real hours at default speed). [3] |

## The player's verb — Metatron

| Assumption | Outcome |
|---|---|
| Whisper/nudge as only influence channel (decision-1) | **Confirmed and re-architected.** All player influence flows through **Metatron**: a singular long-running god-mode agent that watches the sim (via periodic event-stream digests, not per-turn calls), keeps notes, and translates divine intent into agent-comprehensible whispers — judging persuadability, societal impact, and method. Raw player text never enters a villager's context (prompt-injection firewall). [4][5][6] |
| Metatron's authority | **New.** **Gatekeeper** — it can refuse or reshape. And the keystone: **Metatron's charter is the only player-editable prompt in the game.** The meta-game is prompt-engineering your own intermediary; "the prompt is the behavior" survives aimed at your angel, never the villagers. [5] |
| Metatron v1 contract | **New.** Acts only when told: one player prompt = one LLM turn against one aspect of the world (an agent via dream/vision — world-tools parked for post-v1). Default persona: faithful, competent, professional-almost-robotic. Has a soul that starts empty and grows with the reign. Conversation with Metatron is the **primary interface**. [6] |
| Nudge economy ("open question" in concept doc) | **Resolved.** Regenerating charge: **1 per 6 game hours, max 3 banked** — explicitly a rein-the-operator-in mechanic. Forms in v1: **Dream + Omen** ("that binds a society"). [4] |

## The world

| Assumption | Outcome |
|---|---|
| No fail state / "pressure, not war" (concept doc) | **Revised.** Agents **can die** — from collapsed health or starvation (needs: health, food, rest, warmth, morale). No aging (parked). Reversal happened when deathlessness met the conflict-fuel question. [7][8] |
| Miscast valve (decision-1 required topic) | **Revised into an observation goal.** v1 valve = social pressure only; watch where the agents get. Death by neglect is the valve of last resort. Exile-by-vote possible via norms; generational turnover parked. [7][8] |
| Social systems scope | **Revised — all in.** Relationship graph, rumor objects (mutation + provenance), promises/debts ledger, needs, resource economy, norms/votes, seeded secrets: **all seven in v1**, deliberately, as the conflict engine. [8] |
| The gru | **New.** A nocturnal, sight-triggered predator that wounds (neglect kills). Makes night dangerous → shelter/fire/curfew matter → rumor and omen fuel. The Zork homage a terminal game deserves. [9] |
| Map | **Revised.** One village area: wood, water, forage, huntable animals — and **no starting buildings**: Minecraft-style cold start (punch a log, build shelter, light a fire before the first night). Day 1 is dramatic by design. DF-scale maps are an engine requirement, not a v1 feature. [12] |

## Stack & architecture

| Assumption | Outcome |
|---|---|
| Language | **Decided: Go** (daemon posture, concurrency, big-map perf, Bubble Tea TUI; LLM needs are plain HTTP). Python stays for offline prompt experiments. [9→10][12] |
| Persistence | **Decided.** Per-world save directory = **SQLite append-only event log** + snapshots + **flat files bound to the run** (persona.md / soul.md / charter). Never global; runs cleanly separable. [10] |
| Client | **Decided.** Bubble Tea TUI, four panes: **map (default) / chronicle / Metatron console / soul reader**. [12] |

## Candidate Spec Kit specs (larger features needing full specs)

1. **World daemon & time substrate** — tick loop, clock/speeds/pause, event log, save dirs, client protocol, degraded mode.
2. **Agent mind** — persona/soul files, memory window + rerank, planner cadence, conversations, nightly consolidation + firewall validator.
3. **Metatron** — charter (the editable prompt), console conversation, nudge mediation (dream + omen), digests, drama routing.
4. **Social fabric** — relationship graph, rumor objects, promises/debts, secrets, norms/votes.
5. **Village survival sim** — needs, resource chains, building, day/night, the gru, death.

## Parked (deliberately deferred, on the record)

- Metatron world-tools, standing orders / regency (autonomous action while away) [6]
- Aging & generational turnover (persona-blending heirs) [7][8]
- Vector/RAG memory; per-agent custom rerankers as emergent personality [11]
- Multi-village / wilderness expanse; Metatron-mediated removal valve [12][7]
- Drama-router rule for cloud escalation — must be specified inside the Metatron spec [3]
