---
name: agent-mind
description: The thinking layer — immutable personas + accreting souls, event-sourced memories with a deterministic top-K window, and the mind driver injecting planner goals as recorded commands
kind: component
sources:
  - internal/mind/mind.go
  - internal/mind/prompt.go
  - internal/mind/parse.go
  - internal/mind/telemetry.go
  - internal/persona/personas.go
  - internal/persona/files.go
  - internal/scribe/scribe.go
  - internal/sim/memory.go
verified_against: a49d615ec26d41ff14784f5a8f03f89d0e6c96f9
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
stable prompt prefixes. Genesis also seeds Metatron's `charter.md` (the ONE
player-editable prompt, never overwritten once present — [[metatron]]), and the
salience table gains `SalDream` (8) for nudge memories.

**Memories** (`internal/sim/memory.go`): the executor emits `agent.memory_added`
events from a fixed salience table (talk 3★ … death witnessed 10★); the reducer
appends them to `Agent.Memories`. `SelectMemories` is the deterministic working
window: salience halved per game-day of age, top K−2, plus 2 seeded serendipity
picks from the oldest half (bucketed to the planner cadence), presented
reverse-chronologically. K = `WindowK` (10). Prompts never see the whole soul.

**Souls** (`internal/scribe`): an always-on daemon component with its own replica
renders `agents/<name>/soul.md` (dated, starred memories, death freezes the header;
since TASK-9 also a "Who I am becoming" narrative section and a Beliefs section
with confidence + provenance) on memory/death/consolidation events; since TASK-11
it also renders `chronicle.md` from the narrated story ring on `chronicle.entry`
events ([[chronicle]]), and since TASK-13 `village_charter.md` from the norm state
on governance events ([[governance]]). The files are regenerable views — the event
log remains the only truth, so souls survive restarts and travel with the save dir.

**The mind driver** (`internal/mind`): a replica fed by the loop's notify fan-out;
per-agent cadence (1800 ticks, staggered by index) plus triggers — wake, completion
idle, nightfall, first-adjacency encounters (2-game-hour pair cooldown) — floored
by a 5-game-minute per-agent debounce (completion triggers otherwise form a
feedback loop that saturates the local tier). Planner prompts carry a social
context block (bonds, debts, reputation, loudest rumor, and the
last-conversation callback from the record ring — [[social-fabric]], TASK-22) and,
since TASK-13, a "Village law" block (`villageLaw` in prompt.go: active norms with
provenance, exile judgments — second-person for the exile — and the assembly call
while convening; [[governance]]). The driver also runs conversations (see
[[social-fabric]]). Villagers convened to the noon meeting are planner- and
musing-suppressed (`sim.AtMeeting`) until close, their pending triggers left
armed. Since TASK-32 every trigger records its arming stimulus: `arm` takes the
event seq, kept in `pendingSeq` as the causality edge on the eventual telemetry.

Before enqueue, each due agent passes the cognition-horizon gate
(`routeVerdict` in telemetry.go, backed by [[cognition]]'s deterministic
router): a planner thought whose predicted drift exceeds its staleness budget
at the current speed is never attempted — a `cog.outcome{suppressed}` records
the arithmetic, and the reflex floor is the degrade action. Allowed agents are
enqueued as immutable prompt snapshots to a single-flight-per-agent planner
worker — a model call must never block the absorb loop, or the events channel
overflows at high speed and edge triggers are dropped. Each job carries a
`thoughtMeta` identity (job id, decision class, snapshot tick, agent
generation, trigger seq, predicted wall-ms and landing tick from
[[cognition]]'s latency estimate) plus a snapshot of every agent's position
(`agentSnap`) — the assumptions guards are built from. Because the planner
class is `FutureDated`, the prompt opens with `futureDated` (prompt.go): "your
decision will take effect around <landing clock> — plan for then". Each job is
one call (`llm.KindPlanner`, persona system prefix, situation + memory window
suffix, MaxTokens 256); the worker emits `cog.thought` at call start and every
job terminates in exactly one `cog.outcome` (landed, unusable, or —
loop-owned — rejected), riding `InjectSocial` as reducer no-ops
(telemetry.go). The reply's first JSON object is parsed against the goal
vocabulary; the contract now allows either one goal or a guarded plan of at
most `planStepCap` (3) steps (parse.go) — `after_min` becomes a
`GuardAfterTick` guard anchored at the snapshot tick, `for_min` bounds each
step's window (`injectPlan`). Single goals are injected via `Loop.InjectIntent`
— which validates, resolves coordinates deterministically at the tick boundary
(`resolveGoal`), and records `agent.intent_set (source: planner)` +
`agent.thought` — now carrying the landing metadata (`sim.InjectArgs`: Class,
JobID, SnapshotTick, Generation, Predicted/ActualWallMs) and, for `talk_to`,
`GuardTargetAlive` + `GuardTargetPresent` guards built from the job's world
snapshot; the loop owns the landing verdict and its outcome telemetry. A
landing rejection sends the agent index over the `rearm` channel back to the
absorb goroutine — the agent noticed the plan failed and re-thinks at the next
open debounce window, promptly but never hotly. Call and parse failures emit
no intent but always a terminal `cog.outcome{unusable}`; the reflex grace (120
ticks idle) is the floor under every gap, and remains the permanent degraded
mode. The daemon also installs `RecalibrateSignal` as the orchestrator's drift
hook: an estimator spike-rate breach lands as `cog.recalibration_recommended`.

**Musings** (TASK-21): between planner calls each agent has a 15-game-minute
best-effort musing cadence (staggered half a slot off the planner stagger).
A musing is one `llm.KindMusing` call (same situation + memory window, a
plain-sentence system frame, MaxTokens 48) whose reply lands as a single
`agent.thought{source: "musing"}` through `Loop.InjectSocial` — recorded
interiority with zero goal effect. Musings pass the same [[cognition]] router
gate as planners (1 point vs the planner's 3, so they survive far higher
speeds) and carry the same telemetry: `cog.thought` at call start, and a
landed musing rides one `InjectSocial` batch with its
`cog.outcome{landed}` — the musing and its terminal record land atomically.
Single-flight and detached from the absorb
loop; busy tiers ([[llm-orchestrator]]'s `ErrTierBusy` on `BestEffort`
requests) or unusable replies drop the musing — recorded silence, a
`cog.outcome{unusable}`, never a goal. One exception, the
fairness floor: a musing starved past `museStarveWindow` (2 wall-minutes)
drops the `BestEffort` flag and rides the normal queue — a saturated tier
(live finding: back-to-back ~50s planner calls admit zero best-effort work)
costs at most one 48-token call per window instead of total silence.

## Connections

[[executor]] emits memories and runs the intents; [[reflex-policy]] shares
`resolveGoal` and provides the fallback; [[cognition]] owns the decision-class
registry, the router the mind gates on, and the latency estimate behind
predictions and future-dating; [[llm-orchestrator]] carries the calls
(local tier); [[sim-loop]]'s `inject_intent` command is the only door into
deterministic space and since TASK-32 the owner of landing-time validation
(staleness ladder, generation and guard checks); [[event-types]] catalogs the
new events; the [[tui-client]]
souls pane shows each agent's newest memory. [[nightly-consolidation]] digests each
day's memories into the soul at sleep; TASK-8 turned the talk primitive into real
conversations. The mind also hosts the [[chronicle]] narrator (TASK-11): absorb
collects notable events as named log lines and day/night boundaries hand chapters
to a single-flight cloud worker — and the [[governance]] phrasing driver (TASK-13,
`meeting.go`): enacted proposals get one best-effort `llm.KindMeeting` call
rephrasing the template text in the proposer's voice, injected as
`meeting.proposal_rephrased`; every failure leaves the template standing.

## Operational notes

Live-verified against real Ollama: personas visibly steer reasoning (Hazel: "will
charm my way into doing it"), souls accrete and survive restarts, persona hashes
stay intact. Known gap: at `max` speed the mind replica can drop event batches
(overflow policy) — resync-on-overflow is future work; ≤16x is drop-free. Planner
volume at 4x ≈ 16 calls/game-hour for 8 agents, all local-tier.
