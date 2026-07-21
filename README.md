# script-world

A terminal UI, open-world, top-down game on a procedurally generated map — where the
inhabitants are small **AI-programmable agents**.

## The idea

You don't play a character. You populate the world with 10–20 agents, each with its own
avatar, and each fully programmable **via AI prompting**. Give an agent a job, a
personality, a set of priorities — in plain language — and let it loose. Then watch the
world run: agents farm, build, trade, wander, argue, and improvise around each other.

Think **Dwarf Fortress** or **RimWorld**, except instead of indirect management through
menus and zones, you tweak your dwarf by talking to it. The prompt *is* the behavior.

## Core pillars

- **Terminal UI** — the whole game renders in the terminal, top-down.
- **Open world** — a procedurally generated map to explore and settle.
- **Prompt-programmed agents** — every agent's behavior is authored and re-authored
  through AI prompting; the fun is in the scripting.
- **Emergence over scripting** — the game's stories come from agents' AI-driven
  decisions colliding, not from authored quests.

## Status

Grounded and building. The design was interrogated in a Socratic grounding session
(see `docs/design/grounded-assumptions.md`) — script-world is an **ambient, always-on
world**: a daemon simulates the village 24/7; the terminal UI is an attachable client;
all player influence flows through **Metatron**, an editable intermediary agent. The
board lives in `backlog/`; specs in `specs/`.

### What runs today

The time substrate (TASK-2) and the village on top of it: a Go daemon with a
deterministic tick loop (1 tick = 1 game second), an append-only SQLite event log
with snapshots, per-world save directories, and a Unix-socket client protocol —
carrying eight villagers with needs, LLM planner minds, conversations, rumors and
debts, nightly memory consolidation, the nocturnal gru, a cloud-narrated chronicle
(the catch-up mechanism), **Metatron** (TASK-12): the player's sole influence
channel, conversing in the TUI console, mediating dreams and omens on a regenerating
charge economy, governed by `charter.md` — the game's only player-editable prompt —
and **norms and votes** (TASK-13): the village legislates itself at a village meeting
whose hour is a convention (per-world config or emergent), proposals and votes
resolving deterministically off the relationship graph, with the agreed law living in
`village_charter.md` and witnessed violations feeding memories, grudges, and gossip.

```sh
go build ./cmd/scriptworld

scriptworld new demo --seed 42            # create a world (lands in ~/.scriptworld/worlds/demo)
scriptworld start demo                    # detached daemon; the world now runs 24/7
scriptworld ps                            # every running world, machine-wide — name, state, pid,
                                           # tick, game time, speed, LLM on/off (from any directory)
scriptworld status demo                   # tick, game time, speed
scriptworld attach demo                   # watch events live; pause/resume/speed/quit
scriptworld pause demo                    # pause is a player verb (detaching is not)
scriptworld resume demo                   # counterpart of pause
scriptworld speed demo max                # real-time up to as-fast-as-affordable
scriptworld tail demo --follow            # stream the event log
scriptworld ui demo                       # full-screen TUI: map, chronicle, metatron, souls
scriptworld metatron demo "who thrives, who struggles?"   # converse with your angel
scriptworld stop demo                     # graceful stop; kill -9 also resumes lossless
scriptworld help                          # full command list incl. daemon, llm, calibrate
```

Every `<world>` argument above is a **name** — resolved against the default worlds
home (`~/.scriptworld/worlds`, overridable with `SCRIPTWORLD_HOME`) and then a
known-worlds list of custom-path worlds. An explicit **path** (`~/worlds/demo`,
`./demo`, `/srv/games/demo`) still works exactly as before and remains a fully
self-contained, copyable directory — `scriptworld new ~/worlds/demo --seed 42` and
`scriptworld start ~/worlds/demo` are unchanged. `scriptworld ps` is what makes running
several worlds at once safe to reason about: it answers "what's running, and is it
using the shared LLM host?" in one command, with live-proven state (a crashed daemon
never shows as running).

Default speed is 4x: 1 game minute per 15 real seconds; the watchable ladder tops at
32x. `go test -race ./...` covers determinism (same seed → byte-identical history),
crash recovery, the client protocol, and the model-output firewalls.

## The cognition horizon (TASK-32)

A model turn takes real wall time while game time keeps flowing — a ~50s local
planner call is 50 game-seconds of drift at 1x but ~27 game-minutes at 32x. The
cognition horizon (decision-4, `specs/007-cognition-horizon`) scopes **what the
model may decide** by **how stale its answer will be when it lands**:

- Every model-reaching decision class carries a **Fibonacci thought cost**
  (host-independent) and a **staleness budget in game time** (a property of the
  fiction) — `internal/cognition/registry.go`.
- `scriptworld calibrate <dir>` benchmarks your host+model to seconds-per-point
  (`calibration.json`) and prints the horizon your hardware buys ("planner
  suppressed above 16x; musing OK at 32x"). A live estimator follows drift and
  rejects lag spikes; a missing profile means pessimistic bootstrap defaults.
- A **deterministic router** (never a model) gates every call: predicted drift
  over budget → the class degrades (reflex floor, skip, template) and the
  suppression is recorded with its arithmetic.
- **Landing enforcement**: intents carry their snapshot tick, generation, and
  guards; the loop rejects stale/superseded/guard-failed landings — recorded,
  classified (prediction-miss vs world-change), never silent. Prompts are
  future-dated ("your decision takes effect around 09:30") and may return
  guarded plans (≤3 steps; timed guards are the act-at-time-T mechanism).
- **Pause is doctrine**: the world freezes, in-flight minds catch up and land at
  the frozen tick at zero game-tick staleness; no new thought starts.

Read the trail: `sqlite3 world.db "SELECT * FROM events WHERE type LIKE 'cog.%'"`
— every thought terminates in exactly one recorded outcome, chained to its
stimulus (`trigger_seq`), so `stimulus → thought → intent → action` is walkable
from the log alone.
