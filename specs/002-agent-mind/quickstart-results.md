# Quickstart validation results — 2026-07-19

Branch `task-7-agent-mind`, live-verified against a real Ollama instance
(`cogito:3b`) end-to-end through the daemon; automated suites all green under
`-race` (10 packages).

## Scenario A — minds steer bodies (US1) ✅

Fresh world, seed 42, 8 named villagers. Planner calls began on the staggered
cadence and completion triggers; `agent.thought` + `agent.intent_set
{"source":"planner"}` in the log. Goal distribution over the first game-hour:
forage 9, chop 2, build_fire 2, goto_warmth 1 — coherent survival behavior.
**Personas visibly steer**: Hazel (authored: shrewd, charming, allergic to
labor) reasoned "Need warmth and will charm my way into doing it"; Ash
(steady, practical): "Food is low and must ensure survival."

## Scenario B — natures fixed, souls grow (US2) ✅

All 8 `persona.md` files mode `-r--r--r--`; shasum identical after the run
AND after a daemon restart. Sage's `soul.md` accreted "**day 1 06:19** (5★)
Built a fire." and survived restart byte-identical (event-sourced render).
16 memory-worthy events in the first game-hour. Agents with no memorable
happenings stay at "No memories yet" — routine work doesn't mark a soul, by
design.

## Scenario C — degraded mode ✅ (automated)

`TestGarbageOutputFallsToReflex` + `TestDeadModelMeansReflexWorld`: unusable
or dead models produce zero planner events and the reflex floor (2-game-min
grace) keeps the village alive.

## Scenario D — replay is model-free ✅ (automated)

`TestInjectedPlannerIntent`: identical injected timelines produce identical
state hashes; replay applies recorded planner events without any model.

## Notes

- At `max` speed the mind's replica can drop event batches (documented
  design gap: resync-on-overflow is future work); 16x is drop-free.
- First cadence slot lands ~56 real seconds after start at 4x (stagger 225
  ticks/agent) — expect the first thoughts within the first real minute.
