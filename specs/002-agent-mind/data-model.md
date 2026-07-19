# Data Model: Agent Mind v1

**Phase 1 output.** New shapes over the TASK-5 state; file formats in
[contracts/agent-files.md](contracts/agent-files.md), prompt/goal wire shapes in
[contracts/planner-prompt.md](contracts/planner-prompt.md).

## Memory (event-sourced, state-carried)

| Field | Type | Notes |
|---|---|---|
| `text` | string | templated, human-readable ("Talked with Birch") |
| `salience` | int 1..10 | fixed table at emission (see research R2) |
| `tick` | int64 | when it happened; drives recency decay + display time |

Lives in `Agent.Memories []Memory` (appended by the reducer on
`agent.memory_added`); snapshot-carried; rendered into soul.md by the scribe.

## Agent additions

| Field | Type | Notes |
|---|---|---|
| `Memories` | []Memory | full accreted list (bounded later by TASK-9) |
| `IdleSince` | int64 | tick the agent last became idle/awake; reducer-maintained on intent-clearing events, wake, genesis. Reflex fires only after `tick − IdleSince ≥ reflexGraceTicks (120)` |
| `NearDeath` | bool | latch so "nearly died" memories emit once per episode (cleared when health recovers past 400) |

Agent count: **8** (Ash, Birch, Cedar, Rowan, Fern, Hazel, Oak, Sage), each with an
authored persona.

## New / changed events

| Type | Payload | Emitted by | Reducer effect |
|---|---|---|---|
| `agent.memory_added` | `{agent, text, salience}` | executor heuristics (deterministic) | append to `Memories` |
| `agent.thought` | `{agent, text, source}` | `inject_intent` command (planner) | none (chronicle material) |
| `agent.intent_set` | + `source: "reflex"\|"planner"` | reflex / injection | intent installed (unchanged mechanics) |
| intent-clearing events | (existing) | executor | now also stamp `IdleSince` |

`inject_intent` is a **loop command** (like pause/set_speed), not an event type: the
mind calls `Loop.InjectIntent(agent, goal, targetAgent, reason)`; the loop validates,
resolves the target deterministically at the boundary, and records the two events
above. Command events remain the complete input record — replay stays model-free.

## Working-memory window (derived, never stored)

`sim.SelectMemories(a *Agent, seed uint64, tick int64, k int) []Memory` — pure:

1. score each memory: `salience × 0.5^(age / 24 game-hours)`
2. top `k−2` by score (ties: newer wins)
3. 2 serendipity picks from the oldest half, seeded by
   `rngAt(seed, "serendipity", tick/1800, agent)` — stable within a cadence bucket
4. presentation order: reverse-chronological

## Mind driver (daemon-side, no persisted state)

| Field | Notes |
|---|---|
| replica | `sim.State` maintained from the notify stream (DoState at boot) |
| nextDue[agent] | tick of next cadence call (1800-tick stagger by index) |
| pairSeen[a,b] | encounter cooldown (2 game-hours) |
| personas[agent] | file contents loaded at boot (stable prompt prefix) |

Triggers observed from events: `agent.woke`; intent-completion with nothing queued;
`sim.night_started` (all awake); first adjacency of a pair within cooldown. All
scheduling is event-driven — a paused world produces no events, so no cadence fires.

## Scribe (daemon-side, always on)

Separate replica; on `agent.memory_added` / `agent.died`, regenerates that agent's
`soul.md` (header: name, status, day born/died; body: dated memories with salience).
Runs regardless of LLM configuration — souls accrete in model-less worlds too.
