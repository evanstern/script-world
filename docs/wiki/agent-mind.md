---
name: agent-mind
description: The thinking layer — immutable personas + accreting souls, event-sourced memories with a deterministic top-K window, and the mind driver injecting planner goals as recorded commands
kind: component
sources:
  - internal/mind/mind.go
  - internal/mind/prompt.go
  - internal/mind/parse.go
  - internal/persona/personas.go
  - internal/persona/files.go
  - internal/scribe/scribe.go
  - internal/sim/memory.go
verified_against: e9bfdcd6425327ca8e71f188f12c29526802f6b5
---

# Agent mind

TASK-7's thinking layer: eight villagers with authored natures, growing souls, and
planner thoughts from the local model — while replay stays byte-deterministic and
model-free. Three separations do all the work: persona vs soul (fixed vs grown),
events vs files (truth vs view), and mind vs loop (I/O vs determinism).

## How it works

**Personas** (`internal/persona`): eight authored natures, written exactly once by
`scriptworld new` at mode 0444 into `agents/<name>/persona.md` — no post-genesis
write path exists anywhere (the structural half of the persona firewall; the
validation half is [[nightly-consolidation]]'s validator, fed by the authored
`persona.Anchors` and `persona.DriftMarkers`). `Load` reads them as the mind's
stable prompt prefixes.

**Memories** (`internal/sim/memory.go`): the executor emits `agent.memory_added`
events from a fixed salience table (talk 3★ … death witnessed 10★); the reducer
appends them to `Agent.Memories`. `SelectMemories` is the deterministic working
window: salience halved per game-day of age, top K−2, plus 2 seeded serendipity
picks from the oldest half (bucketed to the planner cadence), presented
reverse-chronologically. K = `WindowK` (10). Prompts never see the whole soul.

**Souls** (`internal/scribe`): an always-on daemon component with its own replica
renders `agents/<name>/soul.md` (dated, starred memories, death freezes the header;
since TASK-9 also a "Who I am becoming" narrative section and a Beliefs section
with confidence + provenance) on memory/death/consolidation events. The file is a
regenerable view — the event log remains the only truth, so souls survive restarts
and travel with the save dir.

**The mind driver** (`internal/mind`): a replica fed by the loop's notify fan-out;
per-agent cadence (1800 ticks, staggered by index) plus triggers — wake, completion
idle, nightfall, first-adjacency encounters (2-game-hour pair cooldown) — floored
by a 5-game-minute per-agent debounce (completion triggers otherwise form a
feedback loop that saturates the local tier). Planner prompts carry a social
context block (bonds, debts, reputation, loudest rumor — [[social-fabric]]), and
the driver also runs conversations (see [[social-fabric]]). Due agents are
enqueued as immutable prompt snapshots to a single-flight-per-agent planner
worker — a model call must never block the absorb loop, or the events channel
overflows at high speed and edge triggers are dropped. Each job is one call
(`llm.KindPlanner`, persona system prefix, situation
+ memory window suffix, MaxTokens 256); the first JSON object in the reply is parsed
against the goal vocabulary and injected via `Loop.InjectIntent` — which validates,
resolves coordinates deterministically at the tick boundary (`resolveGoal`), and
records `agent.intent_set (source: planner)` + `agent.thought`. Failures of any kind
(dead model, budget, garbage output, impossible goal) emit nothing; the reflex grace
(120 ticks idle) is the floor under every gap, and remains the permanent degraded
mode.

## Connections

[[executor]] emits memories and runs the intents; [[reflex-policy]] shares
`resolveGoal` and provides the fallback; [[llm-orchestrator]] carries the calls
(local tier); [[sim-loop]]'s `inject_intent` command is the only door into
deterministic space; [[event-types]] catalogs the new events; the [[tui-client]]
souls pane shows each agent's newest memory. [[nightly-consolidation]] digests each
day's memories into the soul at sleep; TASK-8 turned the talk primitive into real
conversations.

## Operational notes

Live-verified against real Ollama: personas visibly steer reasoning (Hazel: "will
charm my way into doing it"), souls accrete and survive restarts, persona hashes
stay intact. Known gap: at `max` speed the mind replica can drop event batches
(overflow policy) — resync-on-overflow is future work; ≤16x is drop-free. Planner
volume at 4x ≈ 16 calls/game-hour for 8 agents, all local-tier.
