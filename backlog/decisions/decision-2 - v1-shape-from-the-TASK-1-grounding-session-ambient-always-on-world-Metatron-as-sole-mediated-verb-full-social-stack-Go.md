---
id: decision-2
title: >-
  v1 shape from the TASK-1 grounding session: ambient always-on world, Metatron
  as sole mediated verb, full social stack, Go
date: '2026-07-19 01:12'
status: Accepted
---
## Context

TASK-1 ran the Socratic grounding session demanded by decision-1: twelve Q/A exchanges
interrogating the Rumor Mill hybrid, logged live in
`topics/script-world-design/101-rumor-mill-grounding/raw-notes.md`. The full
assumption-by-assumption record (confirmed / revised / rejected / new, with citations) is
`docs/design/grounded-assumptions.md` — that document is the detail behind this decision.

## Decision

The v1 target sentence: **an always-on village of 8 agents with fixed personas and growing
souls, all seven social systems live, death by neglect, a gru at night, and one player verb
— nudging (dream + omen) through an editable Metatron — produces, within 30 game days, a
legible story you feel you authored.**

Headline calls (details and citations in grounded-assumptions.md):

1. **Ambient persistent world**, not a session game: sim daemon runs 24/7 (homelab); TUI is
   an attachable Bubble Tea client; default 1 game-min = 15 real-sec (day ≈ 6 real hours);
   pause is a required player verb; speeds real-time ↔ as-fast-as-affordable.
2. **Metatron**: all player influence flows through a singular god-mode gatekeeper agent
   whose charter is the game's ONLY player-editable prompt (the meta-game). V1 contract:
   acts only when told, one prompt = one mediated turn; dream + omen; nudge charges 1 per
   6 game hours, max 3 banked. Metatron has the same persona/soul split as villagers.
3. **Agent mind**: two-layer brain; 30-game-min thought cadence + scene triggers +
   ≤5-turn-each-way conversations; top-K reranked memory window (no embeddings in v1);
   nightly cloud consolidation with a mechanized persona firewall (structural exclusion +
   output validator).
4. **World**: all seven social systems ship in v1 as the conflict engine (relationships,
   rumors, promises/debts, needs, economy, norms/votes, secrets); death by neglect
   (health/food), no aging; the gru hunts at night (wounds, sight-triggered); one-village
   map with a Minecraft-style no-buildings cold start.
5. **Stack**: Go daemon + Bubble Tea TUI; SQLite append-only event log + per-run flat files
   (persona.md / soul.md / charter as literal markdown); inference local-first (Ollama on
   the always-available MacBook via 9router) with a cloud tier, under a hard $100/month.

## Consequences

- The first-pass build tasks (TASK-2 onward) and the five candidate Spec Kit specs named in
  `docs/design/grounded-assumptions.md` §Candidate specs derive from this decision.
- The miscast valve is deliberately an observation goal, not a mechanism: v1 must report
  whether social pressure quarantines a badly-cast agent.
- Parked on the record: Metatron regency/world-tools, aging/generational turnover,
  vector-RAG memory, per-agent rerankers, multi-village maps, the drama-router rule (to be
  specified inside the Metatron spec).
