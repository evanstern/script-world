---
name: agent-mind
description: The thinking layer — immutable personas + accreting souls, event-sourced situated memories with a deterministic top-K window, and the mind driver running each villager cognition through the bounded tool-use loop (spec 017), including the journal tools (spec 019)
kind: component
sources:
  - internal/mind/mind.go
  - internal/mind/prompt.go
  - internal/mind/parse.go
  - internal/mind/handlers.go
  - internal/mind/telemetry.go
  - internal/persona/personas.go
  - internal/persona/files.go
  - internal/scribe/scribe.go
  - internal/sim/memory.go
verified_against: 8ada1050cc5b108790d0e48640dba0b985632e25
---

# Agent mind

TASK-7's thinking layer: eight villagers with authored natures, growing souls, and
planner thoughts from the local model — while replay stays byte-deterministic and
model-free. Three separations do all the work: persona vs soul (fixed vs grown),
events vs files (truth vs view), and mind vs loop (I/O vs determinism).

## How it works

**Personas** (`internal/persona`): eight authored natures, written exactly once by
`promptworld new` at mode 0444 into `agents/<name>/persona.md` — no post-genesis
write path exists anywhere (the structural half of the persona firewall; the
validation half is [[nightly-consolidation]]'s validator, fed by the authored
`persona.Anchors` and `persona.DriftMarkers`). `Load` reads them as the mind's
stable prompt prefixes. Genesis also seeds Metatron's `charter.md` (the ONE
player-editable prompt, never overwritten once present — [[metatron]]), and the
salience table gains `SalDream` (8) for nudge memories. Since spec 019 (US3)
`Genesis` also seeds an empty `agents/<name>/journal.md` beside `soul.md`
(`JournalPath`, files.go) — a regenerable view of the agent's journal state the
scribe rewrites on every `journal.*` event, unlike the once-and-frozen persona
([[agent-journal]]).

**Memories** (`internal/sim/memory.go`): the executor emits `agent.memory_added`
events from a fixed salience table (talk 3★ … death witnessed 10★); the reducer
appends them to `Agent.Memories`. Spec 012's crafting economy added four entries
— `salSpearBroke` (8, the spear that spent its last use), `salOvenBuilt` (7,
village-visible), `salBath` (5, medium and positive), and `salFireOut` (3, a
cold fire nearby is background texture, not formative) — all, like the
pre-existing `SalDream` (8), kept below `GenerationBumpSalience` (9,
[[cognition]]) on purpose: memorable enough to surface in the working window,
never so high they'd interrupt an in-flight generation the way near-death or
exile do. Spec 013's storage economy added two more on the same band:
`salChestBuilt` (7, village-visible, the oven precedent) and `salTaking` (7,
a non-owner withdrawal from a chest — suffered by the owner and witnessed by
neighbors, above the rumor-eligibility floor so the owner's subject-tagged
memory seeds gossip). It also added `memoryEventToned`, a `memoryEvent`
variant for a personal (non-gossip, `Subject: -1`) memory that still carries
an explicit tone — `toneBath` (40), `toneOvenBuilt` (30), and spec 013's
`toneChestBuilt` (20) are positive; the taking itself is recorded (since spec 019 through
`situatedMemoryAboutEvent`, see below) with `theftMemoryTone` (−60, negative) for
both the owner and nearby witnesses, alongside a trust/affection hit on the owner→taker
relationship edge — the existing gossip and relation machinery carries a
chest theft the same way it carries any other trust violation ([[social-fabric]]).
`SelectMemories` is the deterministic working
window: salience halved per game-day of age, top K−2, plus 2 seeded serendipity
picks from the oldest half (bucketed to the planner cadence), presented
reverse-chronologically. K = `WindowK` (10). Prompts never see the whole soul.

Spec 019 (US1) made every sim-emitted episodic memory **situated**: the three
bare constructors are gone — `memoryEvent`/`memoryAboutEvent`/`memoryEventToned`
were replaced (T008b removed the bare forms once every emission site migrated)
by `situatedMemoryEvent`/`situatedMemoryAboutEvent`/`situatedMemoryToned`, so no
memory site can emit unsituated: a site must pick a situated constructor and
therefore a `Where`. Salience/subject/tone semantics are unchanged — this layer
situates, it does not re-weigh. `Where` is a `*sim.MemoryPlace{X,Y,Desc}` baked
at emission by `PlaceAt` (coords always — FR-001; `describePlace`'s deterministic
Manhattan feature scan, radius `placeScanRadius` (2), naming the nearest station
or terrain as a noun phrase — "the fire", "the woods" — or "" when nothing
notable is near); a build completion uses `placeForBuild`/`describePlaceExcept`
to describe its tile WITHOUT the just-placed kind, so "Built a fire" never
resolves to "at the fire" (T024). `Why` is the driving intent's reason verbatim
("" for reflex, and never set on a witness memory — a witness did not drive the
act). `situateText` composes the situated text once, in the grammar order
`<base>[ at <desc> (x,y) | at (x,y)][ — <why>]` (splicing the where-clause before
the base's trailing period), so every call site situates identically and the
scribe never re-derives. The reduced `Memory` gains `Where`/`Why`/`Conv`, all
`omitempty`, copied verbatim by the `agent.memory_added` arm — a pre-019 payload
still produces a pre-019-shaped memory (FR-007/FR-014). Conversation gist
memories are situated differently — `Where` plus a `Conv` transcript ref, text
unchanged ([[social-fabric]]). Villagers also keep a self-authored journal
([[agent-journal]]).

**Souls** (`internal/scribe`): an always-on daemon component with its own replica
renders `agents/<name>/soul.md` (dated, starred memories, death freezes the header;
since TASK-9 also a "Who I am becoming" narrative section and a Beliefs section
with confidence + provenance) on memory/death/consolidation events; since TASK-11
it also renders `chronicle.md` from the narrated story ring on `chronicle.entry`
events ([[chronicle]]), and since TASK-13 `village_charter.md` from the norm state
on governance events ([[governance]]); and since spec 019 (US3) it also renders
each agent's `journal.md` (`renderJournal`, on `journal.entry_written`/
`journal.entry_deleted` — a `jDirty` set kept separate from the soul `dirty`
set, since a journal mutation touches only that one file, souls unaffected;
[[agent-journal]]). The files are regenerable views — the event
log remains the only truth, so souls survive restarts and travel with the save dir.

Spec 019 (T024) kept soul.md's memory lines byte-identical for the common case:
a situated memory already carries its where/why IN THE TEXT (`situateText`, above),
so re-rendering them as a trailing suffix duplicated them ("Built a fire at the
woods (10,40). · at the woods (10,40)"). `memorySuffix` therefore renders ONLY
the one thing NOT in the text — a conversation memory's `Conv` ref, as
`· [conv <id>]`; place and why yield no suffix. A pre-019 memory (and any
non-conversation memory) yields "", so its line is byte-identical to the pre-019
format (FR-006/FR-014/SC-007); the structured `Where`/`Why` fields stay on the
`Memory` for programmatic consumers, only the redundant render is dropped.

**The mind driver** (`internal/mind`): a replica fed by the loop's notify fan-out;
per-agent cadence (1800 ticks, staggered by index; since TASK-44 the stagger is
phase-preserving — every re-arm steps in whole cadence multiples from the agent's
own due via `nextPhasePreservingDue`, never from the current tick, so a shared
stall cannot collapse agents into lockstep) plus triggers — wake, completion
idle, nightfall, first-adjacency encounters (2-game-hour pair cooldown) — floored
by a 5-game-minute per-agent debounce (completion triggers otherwise form a
feedback loop that saturates the local tier). Planner prompts carry a social
context block (bonds, debts, reputation, loudest rumor, and the
last-conversation callback from the record ring — [[social-fabric]], TASK-22;
since TASK-42 scene replies get bounded parse-failure tolerance: `parse.go`'s
`lenientOutcome` repairs the observed unquoted-gist shape with zero extra
calls, and `telemetry.go`'s `cogSceneOutcome` variant carries the failed
reply's bounded `raw` text and a `retried` flag — the base `cogOutcomeEvent`
delegates there with the extras zeroed, keeping every other call site
byte-identical) and,
since TASK-13, a "Village law" block (`villageLaw` in prompt.go: active norms with
provenance, exile judgments — second-person for the exile — and the assembly call
while convening — since TASK-36 all rendered from the event-sourced meeting
convention's clock, with a bare "Village law:" header when none exists;
[[governance]]). The driver also runs conversations (see
[[social-fabric]]). Villagers convened to the daily meeting are planner-suppressed
(`sim.AtMeeting`, checked in `plan()`) until close, their pending triggers left
armed — since musing no longer has a schedule of its own (spec 017, below), this
one gate now also covers it: a convened villager's tool-use loop simply never
runs, so `muse` cannot fire either. Since TASK-32 every trigger records its arming stimulus: `arm` takes the
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
decision will take effect around <landing clock> — plan for then". Each job now
drives a bounded **tool-use loop** (spec 017, `toolloop.Run`, [[tool-loop]])
rather than one bare planner call: `runPlan` builds a `villagerDispatch` (the
job, a wall-clock start, a buffered `CallRecord` sink, and the `doorOutcome`
flag that tells the terminal switch below whether a door already recorded an
outcome) and calls `md.runLoop` — production wires this to `toolloop.Run`
against the concrete `*llm.Orchestrator` (`New`'s `runLoopOverride` variadic
seam installs a scripted driver instead, race-free, for tests that stub the
model through `Submitter`) — with `Kind: llm.KindPlanner`, the persona system
prefix, the situation+memory-window suffix as the loop's `Seed` turn,
`Roster: tool.LoopRosterVillager()`, per-tool handlers from
`md.villagerHandlers`, `MaxRounds: md.loopRounds` (`llm.json`'s
`loop_max_rounds`, normalized), and `MaxTokens: md.plannerTokens` (spec 025,
TASK-72: the operator-tunable `llm.json` `max_tokens.planner` budget, threaded
through `mind.New` like `loopRounds`; default 512, up from the pre-loop 256 —
a tool-era round carries a `tool_use` block alongside any prose, so the budget
grew to avoid truncating a call mid-arguments — with the rationale now living
on the config default in [[llm-orchestrator]]'s `config.go`). `mind.New` also
threads `consolidationTokens` (`max_tokens.consolidation`, default 1024) for
[[nightly-consolidation]]'s Submit. The model
may call read tools first (since spec 019 the villager roster ships two —
`search_journal`/`read_journal`, the first production Read tools, [[agent-journal]])
then must commit to exactly one acting tool — a world verb, `set_plan`, `muse`,
or a journal write/delete — which lands through its existing door; the loop's cardinality rule means every
call after the first acting one is rejected, so "one thought, one act" is
structural now rather than parser-enforced. The retired free-text contract —
`planReply`/`planStepReply`, the parser's `worldGoals`/`validKinds` accept
sets, `validateKindQty`, `plannerReplySchema()`/`plannerSchema`, `parseReply`,
and the golden-prompt fixture (`prompt_golden_test.go`) that pinned it — is
gone: the goal vocabulary, storage kind/qty validation, and the guarded-plan
step cap now live as tool schemas ([[tool-registry]]'s `InputSchema`,
`set_plan`'s authored override) the loop driver itself validates before
dispatch, not a parser the mind ran after the call returned. `systemPrompt`
(prompt.go) no longer renders the goal line/gloss block or the JSON reply
shape — it only frames the choice ("call exactly one acting tool... a world
action, a short plan (set_plan), or a passing thought (muse)"); the tools
themselves carry the vocabulary via their declared name/description/schema.

**Villager tool handlers** (`internal/mind/handlers.go`): every acting tool
wraps an existing landing door, never mutating the world directly — the loop
REQUESTS through the door and translates its verdict into a `toolloop.Outcome`.
`handleWorldVerb(name)` mirrors the pre-loop `InjectIntent` call for one world
verb (the same `InjectArgs` fields, minus the free-text reason, which the tool
era carries via `muse` instead of a per-action field); `talk_to` keeps its
mind-side `buildTalkToGuards` (target alive + present in the job's snapshot
worldview). `handleSetPlan` parses the tool call's `steps` argument into
`[]sim.PlanStep` (`parsePlanSteps`) and lands them via `InjectIntent`'s `Plan`
path, mirroring the retired `injectPlan`. `handleMuse` lands the musing text as
an `agent.thought{source: "musing"}` through `Loop.InjectSocial`, batched
atomically with its `cog.outcome{landed}` — the exact landing the old
scheduled-musing worker used, now driven by the model choosing the `muse` tool
instead of a cadence firing it. Every handler that touches a door sets
`doorOutcome = true` on the dispatch (the door already recorded its own
`cog.outcome`, atomically with the landing/rejection); a handler that refuses
BEFORE touching a door (unknown `talk_to` target, unparseable plan steps)
leaves it false, so `runPlan`'s terminal switch knows to emit its own outcome.

Spec 019 threads a new optional per-action **reason** and wires the four
**journal tools** by name (the `villagerHandlers` switch now dispatches
`write_journal_entry`/`delete_from_journal`/`search_journal`/`read_journal` by
name, ahead of the generic World/Read arms). `reasonArg(call.Args)` reads the
optional `reason` argument (trimmed, defensively capped at `tool.ReasonCapRunes`
= 200) and both `handleWorldVerb` and `handleSetPlan` pass it as
`InjectArgs.Reason` — the intent carries it to completion, where the executor
bakes it into the completion memory's `Why`; the `Loop.InjectIntent` ladder
narrates it as the `agent.thought`. The two journal WRITE handlers
(`handleWriteJournal`/`handleDeleteJournal`) mirror `handleMuse` exactly: they
marshal a `journal.entry_written`/`journal.entry_deleted` event and land it
through `Loop.InjectSocial` batched atomically with a `cog.outcome{landed}`. The
reducer dry-run at the door is the sole gate (the 4000-rune budget for a write,
entry existence for a delete); `journalDoorResult` translates the door result —
success sets `doorOutcome` and returns `VerdictLanded`, a door rejection is
peeled with `errors.Unwrap` so the model sees the gate's reason verbatim as
`VerdictRejectedGate` (the agent can curate and retry), and a non-wrapped error
surfaces as `Err` (infra failure → FR-015 terminal outcome). The two READ
handlers (`handleSearchJournal`/`handleReadJournal`) ground nothing: they read
the cognition's own **journal snapshot** (`d.job.journal`, below) via
`SearchJournal`/`FindJournalEntry`/`JournalEntries`, formatting matches with
`formatJournalEntries` ("#<id> <clock>: <text>") and returning `VerdictReadOK`
(or `VerdictReadError` for an unknown addressed id); zero matches is a
well-formed empty read, never an error. `argInt` reads the integer `entry` id
(float-tolerant, like `argKindQty`). All four are villager-only ([[agent-journal]]).

Reads run in the planner worker goroutine, which must never touch the
absorb-owned replica, so `plan()` snapshots each due agent's journal —
`job.journal = a.Journal.Clone()` — into an immutable per-cognition `*sim.Journal`
carried on `planJob`. The snapshot is what search/read see; writes and deletes
land through the live `InjectSocial` door, not the snapshot.

**Tool-call telemetry** (`telemetry.go`, spec 017 FR-007/T018): every buffered
`CallRecord` the loop's `Record` sink collected lands as a `cog.tool_call`
event (`emitToolCalls`/`toolCallEvent`, via the shared `sim.NewCogToolCallPayload`
constructor also used by [[metatron]]) on EVERY termination path — landed,
rejected, capped, or errored — so a call that never grounded is still queryable
from the log (AC#5). Events are sorted by `Ordinal` before emission (the driver
already buffers them ordinal-dense; sorting here makes the mind's emission
order-independent of buffer order) and ride ONE dedicated `InjectSocial` batch,
separate from the terminal `cog.outcome`. A verdict requiring a non-empty
reason (every `rejected_*` and `read_error`) gets one backfilled from the
verdict name if a handler somehow left it blank, logged as the contract
violation it would be (`verdictRequiresReason`). Since spec 025 (TASK-72)
`runPlan` also surfaces the loop's one in-loop transport retry: when
`res.Retried` is set ([[tool-loop]]'s one-per-run transport retry), it emits a
NON-terminal `cog.outcome` carrying `sim.OutcomeRetried` and the first
failure's reason via `cogOutcomeEvent` — the TASK-42 marker vocabulary, so no
new event type — making every recovery countable from the trail; the terminal
outcome the run earns is still owned by the landing door or the terminal
switch below (the [[tui-client]] decision-trace projection skips the marker so
it never overwrites the earned terminal).

Single goals land via `Loop.InjectIntent` exactly as before — which validates,
resolves coordinates deterministically at the tick boundary (`resolveGoal`),
and records `agent.intent_set (source: planner)` + `agent.thought`, carrying
the landing metadata (`sim.InjectArgs`: Class, JobID, SnapshotTick,
Generation, Predicted/ActualWallMs, and since spec 013 Kind/Qty) and, for
`talk_to`, `GuardTargetAlive` + `GuardTargetPresent` guards — the loop (now
[[tool-loop]]'s `Run`, not `runPlan` itself) owns the round cardinality;
`Loop.InjectIntent`'s landing ladder still owns the landing verdict and its
outcome telemetry, unchanged. `runPlan`'s terminal switch, once the loop
returns, mirrors the pre-loop paths on `res.Term`: a `TermLanded` loop leaves
the sole `cog.outcome` to whichever door landed it (no rearm); a loop that
ended with `d.doorOutcome` true but nothing landed (a rejection, mirroring the
old rejection path) calls `rearmAgent` — the agent noticed the plan failed and
re-thinks at the next open debounce window, promptly but never hotly — with no
outcome added (the door's rejection is the record); and a loop that reached no
door at all (plain text, reads only, an unknown `talk_to` target, an infra
error — `TermModelDone`/`TermCapExhausted`/`TermAdmissionRefused`/`TermCtxDone`)
emits the terminal `cog.outcome{unusable}` itself, with `loopFailReason(res,
err)` naming which termination caused it (FR-015: no failure is silent) — the
reflex grace (120 ticks idle) remains the floor under every gap, and the
permanent degraded mode. The daemon also installs `RecalibrateSignal` as the
orchestrator's drift hook: an estimator spike-rate breach lands as
`cog.recalibration_recommended`.

**Musing** (TASK-21, retired as a scheduled channel by spec 017 R10): a
villager no longer has its own 15-game-minute best-effort cadence, queue,
stagger, or fairness floor (`museCadenceTicks`/`museBusy`/`museDue`/
`museStarveWindow`/`lastMuseOK` and the `muse()` worker are all gone, along
with `KindMusing` itself — [[llm-orchestrator]], [[cognition]]). Musing is now
an ordinary roster tool (`muse`, Expressive, `handleMuse` above) the model may
choose inside its planner tool-use loop — interiority carries the SAME
opportunity cost as any other action, since choosing to muse means not
choosing to act, rather than riding a parallel best-effort channel that could
never compete with real cognition for a tier slot. A musing still lands as a
single `agent.thought{source: "musing"}` batched atomically with its
`cog.outcome{landed}`, and it is still recorded via the loop's normal
`cog.tool_call` trace like any other call — but there is no separate call kind,
cadence, or admission path left to describe; it is one line in
`villagerHandlers`, not a subsystem of its own. `parseMusing` (parse.go)
survives, unrenamed, as the shared one-plain-line parser [[governance]]'s
meeting rephraser also consumes — the scheduled musing it was originally named
for is gone, but its shape (first line, quotes/whitespace stripped, rune-capped)
is still exactly what a plain-text reply needs.

## Connections

[[executor]] emits memories and runs the intents; [[reflex-policy]] shares
`resolveGoal` and provides the fallback; [[cognition]] owns the decision-class
registry, the router the mind gates on, and the latency estimate behind
predictions and future-dating; [[llm-orchestrator]] carries the calls
(local tier); [[tool-loop]] is `runPlan`'s driver (spec 017) — `md.runLoop`
wraps `toolloop.Run`, `tool.LoopRosterVillager()` ([[tool-registry]]) is its
declared roster (since spec 019 including the four journal tools), and
`villagerHandlers` (handlers.go) wraps every acting tool's landing door;
[[agent-journal]] is the spec 019 self-authored notebook the journal handlers
read and write; [[sim-loop]]'s `inject_intent` command is the only door into
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
