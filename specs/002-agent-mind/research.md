# Research: Agent Mind v1

**Phase 0 output** — decisions with rationale; unknowns resolved.

## R1. Where memories live: events → state → files (one truth, two views)

- **Decision**: episodic memories are events (`agent.memory_added {agent, text,
  salience}`) applied into `State.Agents[i].Memories` by the reducer. `soul.md` is a
  regenerated *view* written by a daemon-side scribe from a replica; it is never the
  source of truth.
- **Rationale**: keeps the substrate's event-sourcing contract intact — souls are
  reconstructible from the log (FR-004), snapshots carry them, replay needs no files.
  The scribe pattern reuses the proven TUI replica mechanism.
- **Alternatives considered**: writing soul.md directly from the executor (files as
  truth — breaks replay and crash-consistency); a separate souls table in SQLite
  (second write path; files are the grounded, player-readable choice).

## R2. Memory emission: deterministic executor heuristics

- **Decision**: the executor emits memory events alongside the happenings themselves,
  from a fixed salience table: conversation 3, build fire 5 / shelter 6, hunt 4,
  found-food-while-starving 5, own near-death (health first crossing below 200) 9,
  death witnessed (within radius 8) 10, survived a freezing night 5. Memory text is
  templated ("Talked with Birch by the fire", "Nearly died — cold and starving").
- **Rationale**: pure function of state+tick ⇒ deterministic and replay-safe; the
  interesting *selection* problem is the window, not generation. LLM-authored
  memories arrive with consolidation (TASK-9), which rewrites accreted raw material.
- **Alternatives considered**: planner writes its own memories (non-deterministic
  content entering state — fine as recorded events, but doubles local-tier traffic
  for little v1 value; deferred).

## R3. Working-memory window: deterministic salience×recency + seeded tail

- **Decision**: `SelectMemories(agent, tick, K)` (in `sim`, pure): score = salience ×
  0.5^(age/half-life) with half-life 24 game-hours; take top K−2 by score, then 2
  "serendipity" picks drawn from the oldest half via `rngAt(seed, "serendipity",
  tick-bucket, agent)` — bucketed to the cadence so a prompt retry in the same window
  picks the same tail. Presentation reverse-chronological. K default 10.
- **Rationale**: matches the grounding decision verbatim (top-K, reverse-chron, cheap
  rerank, bottom-of-list mix against stagnation) with zero model cost and full
  determinism (SC-003).
- **Alternatives considered**: embedding/RAG retrieval (parked for v2 by the
  grounding session); LLM rerank (neither cheap nor deterministic).

## R4. Planner scheduling: event-driven mind with a replica

- **Decision**: `mind.Mind` consumes the loop's notify stream (same fan-out as the
  IPC broadcast), maintains its own `sim.State` replica, and checks due-ness per
  agent on every observed batch: cadence = 1800 ticks staggered by agent index;
  triggers = `agent.woke`, intent-completion events when no plan is queued,
  `sim.night_started` (all awake agents), and first-adjacency encounters with a
  2-game-hour pair cooldown. Sleeping/dead agents are skipped.
- **Rationale**: the needs heartbeat guarantees events at least once per game-minute,
  so event-driven scheduling needs no timers and pauses correctly when the world
  pauses (no events → no cadence). The replica gives prompt-building a coherent
  state without touching the loop.
- **Alternatives considered**: polling `DoState` on a wall timer (runs while paused,
  fights the loop for command bandwidth); scheduling inside the loop (puts
  non-deterministic I/O decisions into deterministic space).

## R5. Injection: goals in, coordinates resolved at the boundary

- **Decision**: the planner chooses a *goal* (executor vocabulary + `talk_to
  <name>`); a new loop command `inject_intent` validates (agent alive, awake) and
  resolves the target deterministically at the tick boundary using the same helpers
  the reflex uses (nearest forage/tree/den/build site; `talk_to` → the target
  agent's current tile). It emits `agent.intent_set` with `source: "planner"` plus
  `agent.thought {agent, text}` carrying the model's reason. Unresolvable/unknown
  goals return an error to the mind and emit nothing.
- **Rationale**: keeps every coordinate decision inside deterministic space — the
  model steers, the sim drives (FR-008/009). Reusing reflex resolution means one
  tested path for "nearest X".
- **Alternatives considered**: model outputs coordinates (hallucination-prone,
  invalidates between call and application); mind resolves targets from its replica
  (races the live state by a few ticks).

## R6. Reflex demotion: idle-grace as event-derived state

- **Decision**: `Agent.IdleSince` (tick) is maintained by the reducer on every
  intent-clearing event, wake, and genesis. The reflex only fires when `tick −
  IdleSince ≥ 120` (2 game-minutes). Planner injections normally land well inside
  the grace, so reflexes stay silent while minds are healthy — with no explicit
  "planner enabled" flag anywhere in sim state.
- **Rationale**: the fallback condition must be a pure function of event history for
  replay (SC-005). Whether a planner exists, is down, or is over budget is invisible
  to the sim — all it sees is whether an intent arrived. This is the degraded-mode
  contract from the grounding session, mechanized.
- **Alternatives considered**: config flag in state (ties determinism to a file
  outside the log); disabling reflex entirely when llm.json exists (a dead model
  would freeze the village — violates AC#1's safety property).

## R7. Persona firewall v1: structural + filesystem

- **Decision**: personas are authored constants in `internal/persona`, written once
  by `promptworld new` with mode 0444; no other code path references the file for
  writing. The mind reads persona content at startup (and on soul-dir change) into
  an in-memory map used as the stable prompt prefix.
- **Rationale**: "outside every write path" is the grounding's mechanization (a);
  0444 adds cheap OS-level enforcement. Content validation against drift is
  explicitly TASK-9's half of the firewall.
- **Alternatives considered**: hashing personas into the manifest and verifying at
  boot (adds a failure mode for hand-edited-but-legitimate player tweaks — post-v1
  question; noted for TASK-9).

## R8. Prompt shape: cacheable prefix, strict JSON out

- **Decision**: system = persona + fixed instruction block (stable per agent →
  local-tier prompt cache friendly, and the same structure the cloud tier caches by
  `cache_control` if ever escalated); user = time/needs/location/nearby summary +
  the K-line memory window + "choose one goal" with the vocabulary and a one-line
  JSON schema `{"goal": "...", "target": "...", "reason": "..."}`. Parsing extracts
  the first JSON object; failures are recorded (`agent.thought` with
  `source:"failed"`... no — failures emit nothing into the sim; the mind logs and
  skips). MaxTokens small (256).
- **Rationale**: small models follow terse JSON contracts best; the persona-first
  layout maximizes prefix stability per R8 of the caching guidance.
- **Alternatives considered**: tool/function-calling shapes (Ollama support varies by
  model; plain JSON is the portable floor).
