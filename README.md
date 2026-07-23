# promptworld

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
(see `docs/design/grounded-assumptions.md`) — promptworld is an **ambient, always-on
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
go build ./cmd/promptworld

promptworld new demo --seed 42            # create a world (lands in ~/.promptworld/worlds/demo)
promptworld start demo                    # detached daemon; the world now runs 24/7
promptworld ps                            # every running world, machine-wide — name, state, pid,
                                           # tick, game time, speed, LLM on/off (from any directory)
promptworld status demo                   # tick, game time, speed
promptworld attach demo                   # watch events live; pause/resume/speed/quit
promptworld pause demo                    # pause is a player verb (detaching is not)
promptworld resume demo                   # counterpart of pause
promptworld speed demo max                # real-time up to as-fast-as-affordable
promptworld tail demo --follow            # stream the event log
promptworld ui demo                       # full-screen TUI: map, chronicle, metatron, souls
promptworld metatron demo "who thrives, who struggles?"   # converse with your angel
promptworld stop demo                     # graceful stop; kill -9 also resumes lossless
promptworld help                          # full command list incl. daemon, llm, calibrate
```

Every `<world>` argument above is a **name** — resolved against the default worlds
home (`~/.promptworld/worlds`, overridable with `PROMPTWORLD_HOME`) and then a
known-worlds list of custom-path worlds. An explicit **path** (`~/worlds/demo`,
`./demo`, `/srv/games/demo`) still works exactly as before and remains a fully
self-contained, copyable directory — `promptworld new ~/worlds/demo --seed 42` and
`promptworld start ~/worlds/demo` are unchanged. `promptworld ps` is what makes running
several worlds at once safe to reason about: it answers "what's running, and is it
using the shared LLM host?" in one command, with live-proven state (a crashed daemon
never shows as running).

Default speed is 4x: 1 game minute per 15 real seconds; the watchable ladder tops at
32x. `go test -race ./...` covers determinism (same seed → byte-identical history),
crash recovery, the client protocol, and the model-output firewalls.

## Setup & configuration

**Prerequisites**: Go (build with `go build ./cmd/promptworld`); for AI minds, an
OpenAI-compatible local endpoint (Ollama at `http://localhost:11434/v1` by default,
e.g. `gemma4:12b-mlx`); optionally `ANTHROPIC_API_KEY` in the environment for the
cloud narrative tier. No model configured? The world still runs — reflex-only minds.

**All model traffic is configured in `llm.json`** in the world's save directory
(written by `promptworld new`, read at daemon start). Since spec 024 it is a
**provider registry + per-kind routing chains**:

- `providers` — named model sources (transport `openai_compat` or `anthropic`,
  endpoint, model, pricing, `parallel` worker slots, `tool_mode`,
  `reasoning_effort`, opt-in `endpoint_capacity` for shared-endpoint coordination).
- `routes` — every call kind (planner, conversation, consolidation, narrator, drama,
  metatron, meeting) maps to an **ordered chain** of provider names. Chain order is
  the whole routing policy: first admissible candidate serves; skips happen only for
  mechanical reasons (circuit open / budget / queue full) and are recorded.
- `monthly_budget_usd` — one global ceiling; spend is attributed per provider.
- Pre-024 configs (`local`/`cloud` shape) load unchanged, forever — no migration.

Where a call went and why is always visible: `promptworld status` shows the
per-provider table (health, queue, inflight/slots, contended, spend), and
`promptworld llm <world> <kind> "..."` prints the serving provider and any skips.
`promptworld calibrate <world>` benchmarks each declared provider for the cognition
horizon. Full operator reference: **[docs/llm-providers.md](docs/llm-providers.md)**.

## The cognition horizon (TASK-32)

A model turn takes real wall time while game time keeps flowing — a ~50s local
planner call is 50 game-seconds of drift at 1x but ~27 game-minutes at 32x. The
cognition horizon (decision-4, `specs/007-cognition-horizon`) scopes **what the
model may decide** by **how stale its answer will be when it lands**:

- Every model-reaching decision class carries a **Fibonacci thought cost**
  (host-independent) and a **staleness budget in game time** (a property of the
  fiction) — `internal/cognition/registry.go`.
- `promptworld calibrate <dir>` benchmarks your host+model to seconds-per-point
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

## Tool-calling knobs (`llm.json`, TASK-52)

Cognitions act by calling tools instead of the model replying with free text a mind
hand-parses: the loop presents a roster, dispatches whatever the model calls, and
feeds results back until an action lands or a hard cap trips. Two `llm.json` knobs
tune it:

- **`loop_max_rounds`** — the hard cap on provider rounds a single cognition may
  spend before the driver terminates it. Absent/0 defaults to 8; 1–16 is honored
  verbatim; anything outside that range clamps to it with an operator warning at
  boot — never a boot failure, the same warn-not-error convention as
  `local.parallel`.
- **per-provider `tool_mode`** — `"native"` (the default) speaks the transport's
  first-class function-calling wire: OpenAI-compatible `tool_calls`, or Anthropic
  `tools`. `"json"` engages a provider-agnostic, schema-constrained fallback for
  models whose native function-calling is unreliable: tool declarations move into
  the system prompt and every round is grammar-constrained to a small
  `{tool, args, say}` envelope emulating one tool call. The knob lives on each
  provider entry and is honored only by the `openai_compat` transport — the
  `anthropic` transport is always native and ignores it. (Legacy configs: the old
  `local.tool_mode` / `cloud.tool_mode` fields keep working on the two derived
  providers.)

Flip a model's `tool_mode` to `"json"` when you see the symptoms of unreliable
native function-calling: a run of `rejected_malformed` verdicts in the event log
(the model can't hit the declared argument schema), or the model answering in
plain prose instead of emitting a call at all. Both are the documented cue to
switch that provider's knob — not a code change.

Doctrine, unchanged by which wire shape is in play: a tool call is a request, never
a fact; switching `tool_mode` changes how the request is transported, never what
gets recorded as an event.
